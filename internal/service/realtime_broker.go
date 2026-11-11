package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RealtimeClient struct {
	ClientID       string     `json:"client_id"`
	Module         string     `json:"module"`
	UserID         string     `json:"user_id"`
	Nickname       string     `json:"nickname,omitempty"`
	SessionID      string     `json:"session_id,omitempty"`
	Connected      bool       `json:"connected"`
	ConnectedAt    *time.Time `json:"connected_at,omitempty"`
	DisconnectedAt *time.Time `json:"disconnected_at,omitempty"`
	Subscriptions  int64      `json:"subscriptions"`
	Queued         int64      `json:"queued"`
}
type RealtimeClientsSnapshot struct {
	Status    string           `json:"status"`
	Message   string           `json:"message,omitempty"`
	SampledAt *time.Time       `json:"sampled_at,omitempty"`
	Complete  bool             `json:"complete"`
	Items     []RealtimeClient `json:"items"`
}
type RealtimeBroker struct {
	mu                sync.Mutex
	base, key, secret string
	client            *http.Client
	cache             RealtimeClientsSnapshot
	checked           time.Time
}

func NewRealtimeBroker(base, key, secret string) *RealtimeBroker {
	return &RealtimeBroker{base: strings.TrimRight(strings.TrimSpace(base), "/"), key: key, secret: secret,
		client: &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func brokerTime(raw json.RawMessage) *time.Time {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			v := parsed.UTC()
			return &v
		}
		return nil
	}
	var millis int64
	if json.Unmarshal(raw, &millis) == nil && millis > 0 {
		v := time.UnixMilli(millis).UTC()
		return &v
	}
	return nil
}
func (b *RealtimeBroker) Snapshot(ctx context.Context) RealtimeClientsSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Since(b.checked) < 5*time.Second {
		return b.copy()
	}
	if ctx.Err() != nil {
		return b.copy()
	}
	previousCheck := b.checked
	b.checked = time.Now()
	if b.base == "" || b.key == "" || b.secret == "" {
		b.cache = RealtimeClientsSnapshot{Status: "unconfigured", Message: "尚未配置 EMQX 只读管理接口，实际连接数未知", Items: []RealtimeClient{}}
		return b.copy()
	}
	items, complete, err := b.read(ctx)
	if err != nil {
		if ctx.Err() != nil {
			b.checked = previousCheck
			return b.copy()
		}
		b.cache.Status = "unavailable"
		if b.cache.SampledAt != nil {
			b.cache.Status = "stale"
		}
		b.cache.Message = err.Error()
		return b.copy()
	}
	now := time.Now().UTC()
	b.cache = RealtimeClientsSnapshot{Status: "ok", SampledAt: &now, Complete: complete, Items: items}
	if !complete {
		b.cache.Status = "partial"
		b.cache.Message = "已达到采样上限，连接数量为已采样值，可能还有更多连接"
	}
	return b.copy()
}
func (b *RealtimeBroker) copy() RealtimeClientsSnapshot {
	v := b.cache
	v.Items = append([]RealtimeClient{}, v.Items...)
	return v
}
func (b *RealtimeBroker) read(ctx context.Context) ([]RealtimeClient, bool, error) {
	base, err := url.Parse(b.base)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, false, errors.New("EMQX 管理接口地址无效")
	}
	if base.Path == "" {
		base.Path = "/api/v5"
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/clients"
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	items := []RealtimeClient{}
	seen := map[string]bool{}
	for page := 1; page <= 50; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "limit": {"100"}}
		base.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
		if err != nil {
			return nil, false, errors.New("EMQX 查询请求无效")
		}
		req.SetBasicAuth(b.key, b.secret)
		req.Header.Set("Accept", "application/json")
		resp, err := b.client.Do(req)
		if err != nil {
			return nil, false, errors.New("EMQX 管理接口连接失败或超时")
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				return nil, false, errors.New("EMQX 管理接口鉴权失败，请检查管理 API Key")
			}
			return nil, false, errors.New("EMQX 管理接口暂不可用")
		}
		var result struct {
			Data []struct {
				ClientID        string          `json:"clientid"`
				Username        string          `json:"username"`
				Connected       *bool           `json:"connected"`
				ConnectionState string          `json:"conn_state"`
				ConnectedAt     json.RawMessage `json:"connected_at"`
				DisconnectedAt  json.RawMessage `json:"disconnected_at"`
				Subscriptions   int64           `json:"subscriptions_cnt"`
				Queued          int64           `json:"mqueue_len"`
			} `json:"data"`
			Meta struct {
				HasNext *bool `json:"hasnext"`
				Count   *int  `json:"count"`
			} `json:"meta"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result)
		resp.Body.Close()
		if err != nil || result.Data == nil {
			return nil, false, errors.New("EMQX 返回格式不兼容，请确认使用 v5 管理 API")
		}
		for _, raw := range result.Data {
			module, user, session := realtimeIdentity(raw.Username)
			if module == "unknown" || seen[raw.ClientID] || module == "messages" && !strings.HasPrefix(raw.ClientID, "msg-") || module == "companions" && !strings.HasPrefix(raw.ClientID, "cmp-") {
				continue
			}
			seen[raw.ClientID] = true
			var connected bool
			if raw.Connected != nil {
				connected = *raw.Connected
			} else if raw.ConnectionState == "connected" {
				connected = true
			} else if raw.ConnectionState != "disconnected" && raw.ConnectionState != "idle" {
				return nil, false, errors.New("EMQX 未返回可识别的连接状态")
			}
			items = append(items, RealtimeClient{ClientID: raw.ClientID, Module: module, UserID: user, SessionID: session, Connected: connected, ConnectedAt: brokerTime(raw.ConnectedAt), DisconnectedAt: brokerTime(raw.DisconnectedAt), Subscriptions: raw.Subscriptions, Queued: raw.Queued})
		}
		finished := len(result.Data) < 100
		if result.Meta.HasNext != nil {
			finished = !*result.Meta.HasNext
		} else if result.Meta.Count != nil {
			finished = page*100 >= *result.Meta.Count
		}
		if finished {
			sort.Slice(items, func(i, j int) bool { return items[i].ClientID < items[j].ClientID })
			return items, true, nil
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ClientID < items[j].ClientID })
	return items, false, nil
}
