package regions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/time/rate"
)

var (
	ErrPixabayUnavailable = errors.New("请先配置服务端 PIXABAY_API_KEY")
	ErrPixabayBusy        = errors.New("图片查询繁忙，请稍后重试")
	ErrPixabayExpired     = errors.New("候选图片已过期，请重新查询后导入")
)

// PixabayClient is only called by explicit admin searches/imports, never catalog reads.
// Search responses are cached privately on disk for 24 hours, including across restarts.
type PixabayClient struct {
	key      string
	cacheDir string
	http     *http.Client
	search   chan struct{}
	download chan struct{}
	limit    *rate.Limiter
}

type PixabayCandidate struct {
	ID          string `json:"id"`
	PreviewURL  string `json:"preview_url"`
	SourceURL   string `json:"source_url"`
	Author      string `json:"author"`
	Tags        string `json:"tags"`
	downloadURL string
}

type PixabaySearch struct {
	Query      string             `json:"query"`
	Page       int                `json:"page"`
	HasMore    bool               `json:"has_more"`
	Candidates []PixabayCandidate `json:"candidates"`
}

type pixabayHit struct {
	ID           int64  `json:"id"`
	PageURL      string `json:"pageURL"`
	WebformatURL string `json:"webformatURL"`
	User         string `json:"user"`
	Tags         string `json:"tags"`
}

type pixabayCache struct {
	CreatedAt time.Time    `json:"created_at"`
	TotalHits int          `json:"total_hits"`
	Hits      []pixabayHit `json:"hits"`
}

func NewPixabayClient(key, cacheDir string) *PixabayClient {
	return &PixabayClient{
		key: strings.TrimSpace(key), cacheDir: cacheDir,
		http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !pixabayURL(req.URL.String(), false) {
				return errors.New("invalid image redirect")
			}
			return nil
		}},
		search: make(chan struct{}, 1), download: make(chan struct{}, 2), limit: rate.NewLimiter(rate.Every(time.Second), 2),
	}
}

func (p *PixabayClient) Ready() bool { return p != nil && p.key != "" }

func validPixabayQuery(query string, page int) bool {
	return query != "" && utf8.RuneCountInString(query) <= 100 && strings.IndexFunc(query, unicode.IsControl) < 0 && page >= 1 && page <= 167
}

func (p *PixabayClient) cachePath(query string, page int) string {
	sum := sha256.Sum256([]byte("photos-horizontal-v1\n" + query + "\n" + strconv.Itoa(page)))
	return filepath.Join(p.cacheDir, hex.EncodeToString(sum[:])+".json")
}

func (p *PixabayClient) cached(query string, page int) (pixabayCache, bool) {
	var cached pixabayCache
	raw, err := os.ReadFile(p.cachePath(query, page))
	if err != nil || len(raw) > 1024*1024 || json.Unmarshal(raw, &cached) != nil {
		return cached, false
	}
	age := time.Since(cached.CreatedAt)
	return cached, age >= 0 && age < 24*time.Hour
}

func pixabayURL(raw string, page bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || len(raw) > 2048 {
		return false
	}
	if page {
		return u.Host == "pixabay.com" && !strings.HasPrefix(u.Path, "/get/")
	}
	return u.Host == "cdn.pixabay.com" || u.Host == "pixabay.com" && strings.HasPrefix(u.Path, "/get/")
}

func pixabayResult(query string, page int, cache pixabayCache) PixabaySearch {
	result := PixabaySearch{Query: query, Page: page, HasMore: page*3 < min(cache.TotalHits, 500), Candidates: []PixabayCandidate{}}
	for _, hit := range cache.Hits {
		if hit.ID <= 0 || !pixabayURL(hit.PageURL, true) || !pixabayURL(hit.WebformatURL, false) {
			continue
		}
		// Only the documented webformat substitution is used; never infer original URLs.
		if !strings.Contains(hit.WebformatURL, "_640") {
			continue
		}
		tags := []rune(hit.Tags)
		if len(tags) > 500 {
			tags = tags[:500]
		}
		result.Candidates = append(result.Candidates, PixabayCandidate{
			ID: strconv.FormatInt(hit.ID, 10), PreviewURL: hit.WebformatURL, SourceURL: hit.PageURL,
			Author: hit.User, Tags: string(tags), downloadURL: strings.Replace(hit.WebformatURL, "_640", "_960", 1),
		})
	}
	return result
}

func (p *PixabayClient) Search(ctx context.Context, query string, page int) (PixabaySearch, error) {
	if !p.Ready() {
		return PixabaySearch{}, ErrPixabayUnavailable
	}
	query = strings.TrimSpace(query)
	if !validPixabayQuery(query, page) {
		return PixabaySearch{}, errors.New("关键词须为1至100字，页码无效")
	}
	if cache, ok := p.cached(query, page); ok {
		return pixabayResult(query, page, cache), nil
	}
	select {
	case p.search <- struct{}{}:
		defer func() { <-p.search }()
	default:
		return PixabaySearch{}, ErrPixabayBusy
	}
	if cache, ok := p.cached(query, page); ok {
		return pixabayResult(query, page, cache), nil
	}
	if !p.limit.Allow() {
		return PixabaySearch{}, ErrPixabayBusy
	}
	// Bound private cache growth without evicting still-valid responses and refetching them.
	if err := os.MkdirAll(p.cacheDir, 0o700); err != nil {
		return PixabaySearch{}, errors.New("图片查询缓存暂不可用")
	}
	entries, err := os.ReadDir(p.cacheDir)
	if err != nil {
		return PixabaySearch{}, errors.New("图片查询缓存暂不可用")
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err == nil && time.Since(info.ModTime()) >= 24*time.Hour {
			_ = os.Remove(filepath.Join(p.cacheDir, entry.Name()))
			continue
		}
		count++
	}
	if count >= 512 {
		return PixabaySearch{}, errors.New("今日查询缓存已满，请稍后再试")
	}
	lang := "en"
	if strings.ContainsFunc(query, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		lang = "zh"
	}
	params := url.Values{"key": {p.key}, "q": {query}, "lang": {lang}, "page": {strconv.Itoa(page)}, "per_page": {"3"}, "image_type": {"photo"}, "orientation": {"horizontal"}, "safesearch": {"true"}, "order": {"popular"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://pixabay.com/api/?"+params.Encode(), nil)
	if err != nil {
		return PixabaySearch{}, errors.New("图片查询请求无效")
	}
	response, err := p.http.Do(req)
	if err != nil {
		return PixabaySearch{}, errors.New("Pixabay 连接失败，请稍后重试")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return PixabaySearch{}, ErrPixabayBusy
	}
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return PixabaySearch{}, errors.New("Pixabay 密钥无效或访问受限，请检查服务端配置")
	}
	if response.StatusCode != http.StatusOK {
		return PixabaySearch{}, errors.New("Pixabay 查询失败，请稍后重试")
	}
	var body struct {
		TotalHits int          `json:"totalHits"`
		Hits      []pixabayHit `json:"hits"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&body) != nil || len(body.Hits) > 3 {
		return PixabaySearch{}, errors.New("Pixabay 返回无效内容")
	}
	cache := pixabayCache{CreatedAt: time.Now().UTC(), TotalHits: body.TotalHits, Hits: body.Hits}
	raw, err := json.Marshal(cache)
	if err != nil {
		return PixabaySearch{}, errors.New("图片查询缓存失败")
	}
	tmp, err := os.CreateTemp(p.cacheDir, ".query-*")
	if err != nil {
		return PixabaySearch{}, errors.New("图片查询缓存失败")
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(raw)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil || os.Rename(tmp.Name(), p.cachePath(query, page)) != nil {
		return PixabaySearch{}, errors.New("图片查询缓存失败")
	}
	return pixabayResult(query, page, cache), nil
}

// Download accepts only an ID from a cached search response, never a caller URL.
func (p *PixabayClient) Download(ctx context.Context, query string, page int, id string) ([]byte, string, PixabayCandidate, error) {
	if !p.Ready() {
		return nil, "", PixabayCandidate{}, ErrPixabayUnavailable
	}
	query = strings.TrimSpace(query)
	if !validPixabayQuery(query, page) {
		return nil, "", PixabayCandidate{}, ErrPixabayExpired
	}
	cache, ok := p.cached(query, page)
	if !ok {
		return nil, "", PixabayCandidate{}, ErrPixabayExpired
	}
	var candidate PixabayCandidate
	for _, item := range pixabayResult(query, page, cache).Candidates {
		if item.ID == id {
			candidate = item
			break
		}
	}
	if candidate.ID == "" {
		return nil, "", candidate, ErrPixabayExpired
	}
	select {
	case p.download <- struct{}{}:
		defer func() { <-p.download }()
	default:
		return nil, "", candidate, ErrPixabayBusy
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.downloadURL, nil)
	if err != nil {
		return nil, "", candidate, errors.New("图片地址无效")
	}
	response, err := p.http.Do(req)
	if err != nil {
		return nil, "", candidate, errors.New("图片下载失败，请重试")
	}
	defer response.Body.Close()
	if response.StatusCode == 403 || response.StatusCode == 404 {
		return nil, "", candidate, ErrPixabayExpired
	}
	if response.StatusCode != http.StatusOK {
		return nil, "", candidate, errors.New("图片下载失败，请稍后重试")
	}
	const maxSize = 8 * 1024 * 1024
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxSize+1))
	if err != nil || len(raw) == 0 || len(raw) > maxSize {
		return nil, "", candidate, errors.New("下载图片大小无效")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 1280 || config.Height > 1280 || (format != "jpeg" && format != "png") {
		return nil, "", candidate, errors.New("下载图片格式或尺寸无效")
	}
	return raw, format, candidate, nil
}
