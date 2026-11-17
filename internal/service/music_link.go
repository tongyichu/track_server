package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxMusicBytes = 30 << 20

var musicProbeSlots = make(chan struct{}, 2)
var musicProbeClient = &http.Client{
	Timeout: 12 * time.Second,
	Transport: &http.Transport{
		Proxy: nil, DisableCompression: true, MaxConnsPerHost: 2, MaxIdleConns: 4,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		DialContext:           dialPublicMusicHost,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		if _, err := normalizeMusicLink(req.URL.String()); err != nil {
			return err
		}
		if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("insecure redirect")
		}
		req.Header.Del("Referer")
		return nil
	},
}

func normalizeMusicLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err == nil {
		u.Scheme = strings.ToLower(u.Scheme)
	}
	if err != nil || len(raw) > 4096 || u.User != nil || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return "", fmt.Errorf("%w: 请填写可公开访问的 HTTP/HTTPS 音频直链", ErrMusicInput)
	}
	if port := u.Port(); port != "" && !((u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80")) {
		return "", fmt.Errorf("%w: 音频链接只支持标准 HTTP/HTTPS 端口", ErrMusicInput)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.Contains(host, "%") {
		return "", fmt.Errorf("%w: 音频链接必须为公网地址", ErrMusicInput)
	}
	if ip, err := netip.ParseAddr(host); err == nil && !publicMusicIP(ip) {
		return "", fmt.Errorf("%w: 音频链接必须为公网地址", ErrMusicInput)
	}
	return u.String(), nil
}

func publicMusicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "64:ff9b:1::/48", "fec0::/10", "::/96", "2001::/32", "2002::/16"} {
		if netip.MustParsePrefix(block).Contains(ip) {
			return false
		}
	}
	return true
}

// Resolve and dial the same validated address so a DNS change cannot redirect a probe into the LAN.
func dialPublicMusicHost(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(resolved) == 0 {
		return nil, errors.New("music host unavailable")
	}
	for _, ip := range resolved {
		if !publicMusicIP(ip) {
			return nil, errors.New("music host must be public")
		}
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	for _, ip := range resolved {
		conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
		err = e
	}
	return nil, err
}

type musicLinkInfo struct {
	URL     string
	Bytes   int64
	Version string
}

func probeMusicLink(ctx context.Context, raw string) (musicLinkInfo, error) {
	normalized, err := normalizeMusicLink(raw)
	if err != nil {
		return musicLinkInfo{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	select {
	case musicProbeSlots <- struct{}{}:
		defer func() { <-musicProbeSlots }()
	case <-ctx.Done():
		return musicLinkInfo{}, fmt.Errorf("%w: 链接检查超时，请稍后重试", ErrMusicInput)
	}
	request := func(method string) (*http.Response, error) {
		req, e := http.NewRequestWithContext(ctx, method, normalized, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("Accept", "audio/mpeg, application/octet-stream;q=0.8")
		req.Header.Set("Accept-Encoding", "identity")
		if method == http.MethodGet {
			req.Header.Set("Range", "bytes=0-2047")
		}
		return musicProbeClient.Do(req)
	}
	response, err := request(http.MethodHead)
	if err != nil {
		return musicLinkInfo{}, fmt.Errorf("%w: 链接无法访问或检查超时，请确认可直接公开播放", ErrMusicInput)
	}
	response.Body.Close()
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	knownMP3 := contentType == "audio/mpeg" || contentType == "audio/mp3" || contentType == "audio/x-mpeg"
	length := response.ContentLength
	if response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusBadRequest || response.ContentLength == 0 || (response.StatusCode >= 200 && response.StatusCode < 300 && !knownMP3) {
		response, err = request(http.MethodGet)
		if err != nil {
			return musicLinkInfo{}, fmt.Errorf("%w: 链接无法读取，请确认可公开播放", ErrMusicInput)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
			return musicLinkInfo{}, fmt.Errorf("%w: 链接访问失败（HTTP %d）", ErrMusicInput, response.StatusCode)
		}
		length = response.ContentLength
		if response.StatusCode == http.StatusPartialContent {
			length = 0
			if total := strings.Split(response.Header.Get("Content-Range"), "/"); len(total) == 2 {
				length, _ = strconv.ParseInt(total[1], 10, 64)
			}
		}
		prefix, e := io.ReadAll(io.LimitReader(response.Body, 2048))
		mp3 := len(prefix) >= 3 && string(prefix[:3]) == "ID3"
		if len(prefix) >= 4 && prefix[0] == 255 && prefix[1]&224 == 224 && prefix[1]>>1&3 == 1 && prefix[2]>>2&3 != 3 && prefix[2]>>4 > 0 && prefix[2]>>4 < 15 {
			mp3 = true
		}
		if e != nil || !mp3 {
			return musicLinkInfo{}, fmt.Errorf("%w: 链接未返回 MP3 音频，请勿填写网页或分享页地址", ErrMusicInput)
		}
	} else if response.StatusCode < 200 || response.StatusCode >= 300 {
		return musicLinkInfo{}, fmt.Errorf("%w: 链接访问失败（HTTP %d）", ErrMusicInput, response.StatusCode)
	}
	if length == 0 && response.StatusCode == http.StatusOK {
		return musicLinkInfo{}, fmt.Errorf("%w: 链接返回空内容", ErrMusicInput)
	}
	if length > maxMusicBytes {
		return musicLinkInfo{}, fmt.Errorf("%w: 链接音频大于30MB", ErrMusicInput)
	}
	if length < 0 {
		length = 0
	}
	digest := sha256.Sum256([]byte(normalized + "\n" + response.Header.Get("ETag") + "\n" + response.Header.Get("Last-Modified") + "\n" + strconv.FormatInt(length, 10)))
	return musicLinkInfo{URL: normalized, Bytes: length, Version: hex.EncodeToString(digest[:])}, nil
}
