package regions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const moonshotAPI = "https://api.moonshot.cn/v1"

type ResearchSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type ResearchDraft struct {
	RegionID        string           `json:"region_id"`
	NameEN          string           `json:"name_en,omitempty"`
	Slogan          string           `json:"slogan,omitempty"`
	SloganSourceURL string           `json:"slogan_source_url,omitempty"`
	Description     string           `json:"description,omitempty"`
	Sources         []ResearchSource `json:"sources"`
}

type Researcher struct {
	key          string
	model        string
	client       *http.Client
	moonshotGate chan struct{}
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
	return &Researcher{key: strings.TrimSpace(key), model: strings.TrimSpace(model), client: &http.Client{Timeout: 90 * time.Second}, moonshotGate: make(chan struct{}, 1)}
}

func (r *Researcher) Ready() bool { return r != nil && r.key != "" }

func (r *Researcher) Research(ctx context.Context, node Node, hierarchy []string) (draft *ResearchDraft, researchErr error) {
	started := time.Now()
	var generalTime, sloganTime, completionTime time.Duration
	stage := "configuration"
	defer func() {
		status := "ok"
		if researchErr != nil {
			status = "error"
		}
		log.Printf("[RegionResearch] region_id=%s status=%s stage=%s total_ms=%d general_ms=%d slogan_ms=%d completion_ms=%d", node.RegionID, status, stage, time.Since(started).Milliseconds(), generalTime.Milliseconds(), sloganTime.Milliseconds(), completionTime.Milliseconds())
	}()
	if !r.Ready() {
		return nil, errors.New("城市 AI 联网查询未配置")
	}
	path := strings.Join(hierarchy, "、")
	query := path + " 地区 地标 旅游 官方 简介 英文名称"
	var search researchSearchResponse
	var sloganSearch researchSearchResponse
	stage = "general_search"
	step := time.Now()
	searchErr := r.moonshotJSON(ctx, "/tools/search_pro", map[string]any{"text_query": query, "limit": 6, "timeout_seconds": 30}, &search)
	generalTime = time.Since(step)
	if searchErr != nil {
		return nil, fmt.Errorf("城市资料检索失败: %w", searchErr)
	}
	draft = &ResearchDraft{RegionID: node.RegionID, Sources: []ResearchSource{}}
	var evidence strings.Builder
	sloganSources := make(map[string]string)
	for _, result := range search.SearchResults {
		link := strings.TrimSpace(result.URL)
		if !strings.HasPrefix(link, "https://") || strings.TrimSpace(result.Title) == "" {
			continue
		}
		draft.Sources = append(draft.Sources, ResearchSource{Title: strings.TrimSpace(result.Title), URL: link})
		sloganSources[link] = researchSourceText(result)
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
		stage = "source_validation"
		return nil, errors.New("没有查到可核对的城市资料来源")
	}
	var sloganEvidence strings.Builder
	stage = "slogan_search"
	step = time.Now()
	sloganQuery := path + " 文旅宣传口号 旅游形象宣传语 城市宣传语 经典旅游广告语"
	sloganErr := r.moonshotJSON(ctx, "/tools/search_pro", map[string]any{"text_query": sloganQuery, "limit": 10, "timeout_seconds": 30}, &sloganSearch)
	sloganTime = time.Since(step)
	if sloganErr == nil {
		for _, result := range sloganSearch.SearchResults {
			link := strings.TrimSpace(result.URL)
			if !strings.HasPrefix(link, "https://") || strings.TrimSpace(result.Title) == "" {
				continue
			}
			sourceText := researchSourceText(result)
			sloganSources[link] = sourceText
			draft.Sources = append(draft.Sources, ResearchSource{Title: strings.TrimSpace(result.Title), URL: link})
			fmt.Fprintf(&sloganEvidence, "来源：%s\n链接：%s\n资料：%s\n", result.Title, link, clampRunes(sourceText, 900))
			if sloganEvidence.Len() > 9000 {
				break
			}
		}
	} else {
		log.Printf("[RegionResearch] region_id=%s slogan_search_failed: %v", node.RegionID, sloganErr)
	}
	prompt := fmt.Sprintf(`地区层级：%s
内部编号：%s
现有英文名：%s
现有口号（仅供辨别，不是证据）：%s
现有简介：%s

地区资料：
%s

宣传口号专项检索资料：
%s

请只依据上述资料整理地区信息。英文名用通行译名；简介用中文 100-250 字，突出地理位置、自然与人文地标，避开价格和开放时间等易变信息，不照搬来源段落。
slogan 选择这个地区真实存在、最流行或最经典、最有地域辨识度的一句旅游或城市形象宣传语。官方文旅口号、长期使用的城市品牌语、广泛传播的经典名句和民间旅游宣传语都可选择；官方发布不是必要条件。优先比较多个独立来源中的传播广度、地域辨识度、持续使用情况和现实影响，官方认可度作为辅助。不要只按搜索排名、文案是否好听或一次性活动时间的新旧选择。不能从少量资料推断全国热度，也不能把历史口号冒充现行官方口号。
口号必须在提供的来源标题或正文中逐字可核对，照原文摘录；不得创作、改写或编造来源。排除与当地旅游或城市形象无关的招商、政务、体育、商品广告。slogan_source_url 填写明确载有该口号的来源原始链接，缺少可核对原文才将 slogan 和来源留空。其他字段无法确定时留空。只输出 JSON 对象，键固定为 name_en、slogan、slogan_source_url、description。`, path, node.RegionID, node.NameEN, node.Slogan, node.Description, evidence.String(), sloganEvidence.String())
	var completion struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	completionInput := map[string]any{
		"model": r.model, "temperature": 1, "max_tokens": 32768,
		"response_format": map[string]string{"type": "json_object"},
		"messages":        []map[string]string{{"role": "system", "content": "你是严谨的地区资料编辑，只依据提供的来源写作，不猜测。"}, {"role": "user", "content": prompt}},
	}
	if r.model == "kimi-k2.6" || r.model == "kimi-k2.5" {
		completionInput["thinking"] = map[string]string{"type": "disabled"}
		completionInput["max_tokens"] = 4096
		completionInput["temperature"] = 0.6
		completionInput["top_p"] = 0.95
	}
	stage = "completion"
	step = time.Now()
	if err := r.moonshotJSON(ctx, "/chat/completions", completionInput, &completion); err != nil {
		completionTime = time.Since(step)
		return nil, fmt.Errorf("城市资料整理失败: %w", err)
	}
	completionTime = time.Since(step)
	if len(completion.Choices) == 0 {
		return nil, errors.New("城市 AI 未返回资料")
	}
	choice := completion.Choices[0]
	content := strings.TrimSpace(choice.Message.Content)
	if choice.FinishReason == "length" {
		log.Printf("[RegionResearch] region_id=%s truncated_completion content_bytes=%d", node.RegionID, len(content))
		stage = "completion_format"
		return nil, errors.New("城市 AI 输出被截断，请重试")
	}
	var fields struct {
		NameEN          string `json:"name_en"`
		Slogan          string `json:"slogan"`
		SloganSourceURL string `json:"slogan_source_url"`
		Description     string `json:"description"`
	}
	if err := json.Unmarshal([]byte(content), &fields); err != nil || !strings.HasPrefix(content, "{") {
		log.Printf("[RegionResearch] region_id=%s invalid_completion finish_reason=%q content_bytes=%d decode_error=%v", node.RegionID, choice.FinishReason, len(content), err)
		stage = "completion_format"
		return nil, errors.New("城市 AI 返回格式无效")
	}
	draft.NameEN = strings.TrimSpace(fields.NameEN)
	if sourceText, ok := sloganSources[strings.TrimSpace(fields.SloganSourceURL)]; ok && sourcedRegionSlogan(sourceText, fields.Slogan) {
		draft.Slogan = strings.TrimSpace(fields.Slogan)
		draft.SloganSourceURL = strings.TrimSpace(fields.SloganSourceURL)
	}
	draft.Description = strings.TrimSpace(fields.Description)
	if draft.Slogan == "" {
		stage = "slogan_fallback"
		step = time.Now()
		fallback, fallbackErr := r.researchSloganFallback(ctx, path, sloganSources)
		sloganTime += time.Since(step)
		if fallbackErr != nil {
			log.Printf("[RegionResearch] region_id=%s slogan_fallback_failed: %v", node.RegionID, fallbackErr)
		} else if fallback != nil {
			draft.Sources = append(draft.Sources, fallback.Sources...)
			if sourceText, ok := sloganSources[fallback.SourceURL]; ok && sourcedRegionSlogan(sourceText, fallback.Slogan) {
				draft.Slogan = fallback.Slogan
				draft.SloganSourceURL = fallback.SourceURL
			}
		}
	}
	if utf8.RuneCountInString(draft.NameEN) > 200 || utf8.RuneCountInString(draft.Slogan) > 500 || utf8.RuneCountInString(draft.Description) > 8000 {
		return nil, errors.New("城市 AI 返回的资料过长")
	}
	stage = "done"
	return draft, nil
}

type fallbackSlogan struct {
	Slogan    string           `json:"slogan"`
	SourceURL string           `json:"slogan_source_url"`
	Sources   []ResearchSource `json:"-"`
}

func (r *Researcher) researchSloganFallback(ctx context.Context, path string, sourceTexts map[string]string) (*fallbackSlogan, error) {
	var search researchSearchResponse
	query := path + " 最火 城市品牌口号 经典旅游广告语 文旅推介 旅游节 城市形象片"
	if err := r.moonshotJSON(ctx, "/tools/search_pro", map[string]any{"text_query": query, "limit": 10, "timeout_seconds": 30}, &search); err != nil {
		return nil, err
	}
	var evidence strings.Builder
	result := &fallbackSlogan{}
	for _, item := range search.SearchResults {
		link := strings.TrimSpace(item.URL)
		if !strings.HasPrefix(link, "https://") || strings.TrimSpace(item.Title) == "" {
			continue
		}
		content := researchSourceText(item)
		sourceTexts[link] = content
		result.Sources = append(result.Sources, ResearchSource{Title: strings.TrimSpace(item.Title), URL: link})
		fmt.Fprintf(&evidence, "来源：%s\n链接：%s\n资料：%s\n", item.Title, link, clampRunes(content, 900))
		if evidence.Len() > 9000 {
			break
		}
	}
	if len(result.Sources) == 0 {
		return result, nil
	}
	prompt := fmt.Sprintf(`地区：%s
首轮没有找到可核对的代表性宣传语，请根据以下补充资料再查一次。
%s
选择当地真实存在、最有传播度和地域辨识度的一句旅游或城市形象宣传语。官方、媒体长期使用、民间经典说法都可以；不要偏爱一次性活动主题，不要编造流行程度或官方身份。只从来源标题或正文逐字摘录，slogan_source_url 必须是明确载有该句的上述链接。确实没有可核对原文时留空。只输出 JSON 对象，键为 slogan、slogan_source_url。`, path, evidence.String())
	input := map[string]any{
		"model": r.model, "temperature": 0.6, "max_tokens": 1024,
		"response_format": map[string]string{"type": "json_object"},
		"messages":        []map[string]string{{"role": "system", "content": "你是严谨的地区资料编辑，只依据给出的来源摘录，不猜测。"}, {"role": "user", "content": prompt}},
	}
	if r.model == "kimi-k2.6" || r.model == "kimi-k2.5" {
		input["thinking"] = map[string]string{"type": "disabled"}
	}
	var completion struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := r.moonshotJSON(ctx, "/chat/completions", input, &completion); err != nil {
		return result, err
	}
	if len(completion.Choices) == 0 || completion.Choices[0].FinishReason == "length" {
		return result, errors.New("补充检索的口号整理未完成")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if !strings.HasPrefix(content, "{") || json.Unmarshal([]byte(content), result) != nil {
		return result, errors.New("补充检索的口号格式无效")
	}
	result.Slogan = strings.TrimSpace(result.Slogan)
	result.SourceURL = strings.TrimSpace(result.SourceURL)
	return result, nil
}

func (r *Researcher) moonshotJSON(ctx context.Context, endpoint string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if r.moonshotGate != nil {
		select {
		case r.moonshotGate <- struct{}{}:
			defer func() { <-r.moonshotGate }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	concurrencyRetries := 0
	timeoutRetries := 0
	for {
		requestCtx := ctx
		cancel := func() {}
		if endpoint == "/chat/completions" {
			requestCtx, cancel = context.WithTimeout(ctx, 45*time.Second)
		}
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, moonshotAPI+endpoint, bytes.NewReader(body))
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Authorization", "Bearer "+r.key)
		req.Header.Set("Content-Type", "application/json")
		err = r.doJSON(req, output)
		cancel()
		if err == nil {
			return nil
		}
		if endpoint == "/chat/completions" && errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil && timeoutRetries < 1 {
			timeoutRetries++
			log.Printf("[RegionResearch] moonshot_timeout_retry endpoint=%s attempt=%d", endpoint, timeoutRetries)
			continue
		}
		var statusErr *researchHTTPError
		if !errors.As(err, &statusErr) || statusErr.code != http.StatusTooManyRequests || !strings.Contains(strings.ToLower(statusErr.message), "concurrency") || concurrencyRetries == 3 {
			return err
		}
		wait := time.Duration(1<<concurrencyRetries) * time.Second
		concurrencyRetries++
		log.Printf("[RegionResearch] moonshot_concurrency_retry endpoint=%s attempt=%d wait_ms=%d", endpoint, concurrencyRetries, wait.Milliseconds())
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

type researchHTTPError struct {
	code    int
	message string
}

func (e *researchHTTPError) Error() string {
	if e.message == "" {
		return fmt.Sprintf("HTTP %d", e.code)
	}
	return fmt.Sprintf("HTTP %d: %s", e.code, e.message)
}

func (r *Researcher) doJSON(req *http.Request, output any) error {
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var providerError struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			Message string `json:"message"`
		}
		if raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096)); err == nil && json.Unmarshal(raw, &providerError) == nil {
			message := providerError.Error.Message
			if message == "" {
				message = providerError.Message
			}
			message = strings.Join(strings.Fields(message), " ")
			return &researchHTTPError{code: resp.StatusCode, message: clampRunes(message, 240)}
		}
		return &researchHTTPError{code: resp.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(output)
}

func researchSourceText(result researchSearchResult) string {
	var text strings.Builder
	text.WriteString(result.Title)
	text.WriteString("\n")
	text.WriteString(result.Snippet)
	for _, chunk := range result.Chunks {
		text.WriteString("\n")
		text.WriteString(chunk.Text)
	}
	return text.String()
}

func sourcedRegionSlogan(sourceText, slogan string) bool {
	slogan = strings.TrimSpace(slogan)
	if utf8.RuneCountInString(slogan) < 4 || utf8.RuneCountInString(slogan) > 80 {
		return false
	}
	text := strings.Join(strings.Fields(sourceText), "")
	value := strings.Join(strings.Fields(slogan), "")
	return strings.Contains(text, value)
}

func clampRunes(value string, max int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	return string([]rune(value)[:max])
}
