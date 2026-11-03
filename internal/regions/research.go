package regions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const commonsAPI = "https://commons.wikimedia.org/w/api.php"
const moonshotAPI = "https://api.moonshot.cn/v1"

type ResearchSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type PhotoCandidate struct {
	FileTitle   string `json:"file_title"`
	PreviewURL  string `json:"preview_url"`
	SourceURL   string `json:"source_url"`
	Attribution string `json:"attribution,omitempty"`
	License     string `json:"license"`
	LicenseURL  string `json:"license_url,omitempty"`
}

type ResearchDraft struct {
	RegionID        string           `json:"region_id"`
	NameEN          string           `json:"name_en,omitempty"`
	Slogan          string           `json:"slogan,omitempty"`
	SloganSourceURL string           `json:"slogan_source_url,omitempty"`
	Description     string           `json:"description,omitempty"`
	Sources         []ResearchSource `json:"sources"`
	Photos          []PhotoCandidate `json:"photos"`
}

type Researcher struct {
	key    string
	model  string
	client *http.Client
}

type researchSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Chunks  []struct {
		Text string `json:"text"`
	} `json:"chunks"`
}

type researchSearchResponse struct {
	SearchResults []researchSearchResult `json:"search_results"`
}

func NewResearcher(key, model string) *Researcher {
	if model == "" {
		model = "kimi-k2.6"
	}
	return &Researcher{key: strings.TrimSpace(key), model: strings.TrimSpace(model), client: &http.Client{Timeout: 90 * time.Second}}
}

func (r *Researcher) Ready() bool { return r != nil && r.key != "" }

func (r *Researcher) Research(ctx context.Context, node Node, hierarchy []string) (*ResearchDraft, error) {
	if !r.Ready() {
		return nil, errors.New("城市 AI 联网查询未配置")
	}
	path := strings.Join(hierarchy, "、")
	query := path + " 地区 地标 旅游 官方 简介 英文名称"
	var search researchSearchResponse
	if err := r.moonshotJSON(ctx, "/tools/search_pro", map[string]any{"text_query": query, "limit": 6, "timeout_seconds": 30}, &search); err != nil {
		return nil, fmt.Errorf("城市资料检索失败: %w", err)
	}
	draft := &ResearchDraft{RegionID: node.RegionID, Sources: []ResearchSource{}, Photos: []PhotoCandidate{}}
	var evidence strings.Builder
	for _, result := range search.SearchResults {
		link := strings.TrimSpace(result.URL)
		if !strings.HasPrefix(link, "https://") || strings.TrimSpace(result.Title) == "" {
			continue
		}
		draft.Sources = append(draft.Sources, ResearchSource{Title: strings.TrimSpace(result.Title), URL: link})
		fmt.Fprintf(&evidence, "来源：%s\n链接：%s\n摘要：%s\n", result.Title, link, clampRunes(result.Snippet, 500))
		for _, chunk := range result.Chunks {
			fmt.Fprintf(&evidence, "资料：%s\n", clampRunes(chunk.Text, 500))
			if evidence.Len() > 8000 {
				break
			}
		}
		if evidence.Len() > 8000 {
			break
		}
	}
	if len(draft.Sources) == 0 {
		return nil, errors.New("没有查到可核对的城市资料来源")
	}
	sloganSources := make(map[string]string)
	var sloganEvidence strings.Builder
	var sloganSearch researchSearchResponse
	sloganQuery := path + " 文旅 旅游 宣传口号 旅游形象口号 旅游宣传语"
	if err := r.moonshotJSON(ctx, "/tools/search_pro", map[string]any{"text_query": sloganQuery, "limit": 8, "timeout_seconds": 30}, &sloganSearch); err == nil {
		for _, result := range sloganSearch.SearchResults {
			link := strings.TrimSpace(result.URL)
			if !strings.HasPrefix(link, "https://") || strings.TrimSpace(result.Title) == "" {
				continue
			}
			var sourceText strings.Builder
			sourceText.WriteString(result.Snippet)
			for _, chunk := range result.Chunks {
				sourceText.WriteString("\n")
				sourceText.WriteString(chunk.Text)
			}
			sloganSources[link] = sourceText.String()
			draft.Sources = append(draft.Sources, ResearchSource{Title: strings.TrimSpace(result.Title), URL: link})
			fmt.Fprintf(&sloganEvidence, "来源：%s\n链接：%s\n资料：%s\n", result.Title, link, clampRunes(sourceText.String(), 900))
			if sloganEvidence.Len() > 9000 {
				break
			}
		}
	}
	prompt := fmt.Sprintf("地区层级：%s\n内部编号：%s\n现有英文名：%s\n现有口号（仅供辨别，不是证据）：%s\n现有简介：%s\n\n地区资料：\n%s\n\n文旅口号专项检索资料：\n%s\n请只依据资料生成适合户外目的地客户端的地区资料。英文名用通行官方译名；简介用中文 100-250 字，突出地理位置、自然与人文地标，避开价格、开放时间等易变信息。slogan 只能填写该地区文旅部门或旅游推广中实际使用、有来源明确记载的旅游宣传口号/旅游形象口号，须照来源原文摘录，不创作、不改写。城市精神、文明创建、招商引资、政务施政、体育赛事、品牌商品等其他口号一律不能用；不能确认是文旅口号或没有可核对来源时填空字符串。slogan_source_url 填写文旅口号专项检索资料中明确载有该口号的原始链接，slogan 为空时也留空。其他字段资料不能确定时填空字符串。只输出 JSON 对象，键固定为 name_en、slogan、slogan_source_url、description。简介不要引用整段原文。", path, node.RegionID, node.NameEN, node.Slogan, node.Description, evidence.String(), sloganEvidence.String())
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	completionInput := map[string]any{
		"model": r.model, "temperature": 1, "max_tokens": 32768,
		"messages": []map[string]string{{"role": "system", "content": "你是严谨的地区资料编辑，只依据提供的来源写作，不猜测。"}, {"role": "user", "content": prompt}},
	}
	if r.model == "kimi-k2.6" || r.model == "kimi-k2.5" {
		completionInput["thinking"] = map[string]string{"type": "disabled"}
		completionInput["max_tokens"] = 4096
	}
	if err := r.moonshotJSON(ctx, "/chat/completions", completionInput, &completion); err != nil {
		return nil, fmt.Errorf("城市资料整理失败: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("城市 AI 未返回资料")
	}
	content := completion.Choices[0].Message.Content
	start, end := strings.IndexByte(content, '{'), strings.LastIndexByte(content, '}')
	if start < 0 || end <= start {
		return nil, errors.New("城市 AI 返回格式无效")
	}
	var fields struct {
		NameEN          string `json:"name_en"`
		Slogan          string `json:"slogan"`
		SloganSourceURL string `json:"slogan_source_url"`
		Description     string `json:"description"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &fields); err != nil {
		return nil, errors.New("城市 AI 返回格式无效")
	}
	draft.NameEN = strings.TrimSpace(fields.NameEN)
	if sourceText, ok := sloganSources[strings.TrimSpace(fields.SloganSourceURL)]; ok && sourcedTourismSlogan(sourceText, fields.Slogan) {
		draft.Slogan = strings.TrimSpace(fields.Slogan)
		draft.SloganSourceURL = strings.TrimSpace(fields.SloganSourceURL)
	}
	draft.Description = strings.TrimSpace(fields.Description)
	if utf8.RuneCountInString(draft.NameEN) > 200 || utf8.RuneCountInString(draft.Slogan) > 500 || utf8.RuneCountInString(draft.Description) > 8000 {
		return nil, errors.New("城市 AI 返回的资料过长")
	}
	photoQueries := []string{}
	if draft.NameEN != "" {
		photoQueries = append(photoQueries, draft.NameEN+" skyline", draft.NameEN+" landscape", draft.NameEN+" landmark")
	}
	photoQueries = append(photoQueries, node.Name+" 风景", node.Name+" 地标", node.Name+" 城市")
	seen := make(map[string]bool)
	for _, term := range photoQueries {
		if len(draft.Photos) == 3 {
			break
		}
		searchCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		candidates, err := r.searchCommons(searchCtx, term)
		cancel()
		if err != nil {
			continue // Text research remains usable when Commons is unavailable.
		}
		for _, candidate := range candidates {
			if seen[candidate.FileTitle] {
				continue
			}
			seen[candidate.FileTitle] = true
			draft.Photos = append(draft.Photos, candidate)
			break
		}
	}
	return draft, nil
}

func (r *Researcher) moonshotJSON(ctx context.Context, endpoint string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, moonshotAPI+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")
	return r.doJSON(req, output)
}

func (r *Researcher) doJSON(req *http.Request, output any) error {
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(output)
}

type commonsPage struct {
	Title     string `json:"title"`
	Index     int    `json:"index"`
	ImageInfo []struct {
		URL            string `json:"url"`
		ThumbURL       string `json:"thumburl"`
		DescriptionURL string `json:"descriptionurl"`
		Mime           string `json:"mime"`
		Width          int    `json:"width"`
		Height         int    `json:"height"`
		ExtMetadata    map[string]struct {
			Value string `json:"value"`
		} `json:"extmetadata"`
	} `json:"imageinfo"`
}

func (r *Researcher) commonsQuery(ctx context.Context, params url.Values) ([]commonsPage, error) {
	params.Set("action", "query")
	params.Set("format", "json")
	params.Set("prop", "imageinfo")
	params.Set("iiprop", "url|size|mime|extmetadata")
	params.Set("iiurlwidth", "1600")
	params.Set("iiextmetadatafilter", "LicenseShortName|LicenseUrl|Artist|Credit")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, commonsAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TrackServer/1.0 (https://github.com/tongyichu/track_server; admin city photo research)")
	var response struct {
		Query struct {
			Pages map[string]commonsPage `json:"pages"`
		} `json:"query"`
	}
	if err := r.doJSON(req, &response); err != nil {
		return nil, err
	}
	pages := make([]commonsPage, 0, len(response.Query.Pages))
	for _, page := range response.Query.Pages {
		pages = append(pages, page)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Index < pages[j].Index })
	return pages, nil
}

func (r *Researcher) searchCommons(ctx context.Context, term string) ([]PhotoCandidate, error) {
	params := url.Values{"generator": {"search"}, "gsrnamespace": {"6"}, "gsrlimit": {"25"}, "gsrsearch": {term}}
	pages, err := r.commonsQuery(ctx, params)
	if err != nil {
		return nil, err
	}
	return commonsCandidates(pages), nil
}

func (r *Researcher) CommonsPhoto(ctx context.Context, title string) (PhotoCandidate, error) {
	if !strings.HasPrefix(title, "File:") || len(title) > 300 {
		return PhotoCandidate{}, errors.New("图片来源标识无效")
	}
	pages, err := r.commonsQuery(ctx, url.Values{"titles": {title}})
	if err != nil {
		return PhotoCandidate{}, err
	}
	for _, candidate := range commonsCandidates(pages) {
		if candidate.FileTitle == title {
			return candidate, nil
		}
	}
	return PhotoCandidate{}, errors.New("图片已不可用或缺少可核对的许可信息")
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)
var freePhotoLicense = regexp.MustCompile(`^(?:CC BY(?:-SA)?(?: [0-9.]+)?|CC0(?: 1\.0)?)$`)

func plainMetadata(value string) string {
	return strings.Join(strings.Fields(html.UnescapeString(htmlTag.ReplaceAllString(value, ""))), " ")
}

func commonsCandidates(pages []commonsPage) []PhotoCandidate {
	out := make([]PhotoCandidate, 0, len(pages))
	for _, page := range pages {
		if len(page.ImageInfo) == 0 || !strings.HasPrefix(page.Title, "File:") {
			continue
		}
		info := page.ImageInfo[0]
		if (info.Mime != "image/jpeg" && info.Mime != "image/png") || info.Width < 1000 || info.Height < 500 || float64(info.Width)/float64(info.Height) < 1.25 {
			continue
		}
		lower := strings.ToLower(page.Title)
		if strings.Contains(lower, "logo") || strings.Contains(lower, "flag") || strings.Contains(lower, "map") || strings.Contains(lower, "poster") || strings.Contains(lower, "diagram") {
			continue
		}
		license := plainMetadata(info.ExtMetadata["LicenseShortName"].Value)
		if !freePhotoLicense.MatchString(license) && !strings.HasPrefix(strings.ToLower(license), "public domain") && license != "PD" {
			continue
		}
		preview := info.ThumbURL
		if preview == "" {
			preview = info.URL
		}
		if !isCommonsImageURL(preview) || !strings.HasPrefix(info.DescriptionURL, "https://commons.wikimedia.org/") {
			continue
		}
		attribution := plainMetadata(info.ExtMetadata["Artist"].Value)
		if attribution == "" {
			attribution = plainMetadata(info.ExtMetadata["Credit"].Value)
		}
		out = append(out, PhotoCandidate{FileTitle: page.Title, PreviewURL: preview, SourceURL: info.DescriptionURL, Attribution: clampRunes(attribution, 300), License: license, LicenseURL: normalizeHTTPSURL(plainMetadata(info.ExtMetadata["LicenseUrl"].Value))})
	}
	return out
}

func isCommonsImageURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() == "upload.wikimedia.org" && u.User == nil && strings.HasPrefix(u.Path, "/wikipedia/commons/")
}

func normalizeHTTPSURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	} else if strings.HasPrefix(raw, "http://") {
		raw = "https://" + strings.TrimPrefix(raw, "http://")
	}
	if !strings.HasPrefix(raw, "https://") {
		return ""
	}
	return raw
}

func sourcedTourismSlogan(sourceText, slogan string) bool {
	slogan = strings.TrimSpace(slogan)
	if utf8.RuneCountInString(slogan) < 4 || utf8.RuneCountInString(slogan) > 80 {
		return false
	}
	text := strings.Join(strings.Fields(sourceText), "")
	value := strings.Join(strings.Fields(slogan), "")
	index := strings.Index(text, value)
	if index < 0 {
		return false
	}
	runes := []rune(text)
	start := utf8.RuneCountInString(text[:index]) - 100
	if start < 0 {
		start = 0
	}
	end := start + utf8.RuneCountInString(value) + 200
	if end > len(runes) {
		end = len(runes)
	}
	nearby := string(runes[start:end])
	for _, keyword := range []string{"文旅宣传", "文旅口号", "文旅形象", "文旅品牌", "旅游宣传", "旅游口号", "旅游形象", "旅游品牌"} {
		if strings.Contains(nearby, keyword) {
			return true
		}
	}
	return false
}

func clampRunes(value string, max int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	return string([]rune(value)[:max])
}
