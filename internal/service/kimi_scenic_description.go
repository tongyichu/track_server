package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/models"
)

const (
	kimiAPIBaseURL       = "https://api.moonshot.cn/v1"
	maxScenicDescription = 200
)

type ScenicSpotDescriptionGenerator interface {
	Generate(context.Context, *models.ScenicSpot) (string, error)
}

type KimiScenicSpotDescriptionGenerator struct {
	apiKey string
	model  string
	client *http.Client
}

func NewKimiScenicSpotDescriptionGenerator(apiKey, model string) *KimiScenicSpotDescriptionGenerator {
	if strings.TrimSpace(model) == "" {
		model = "kimi-k2.6"
	}
	return &KimiScenicSpotDescriptionGenerator{
		apiKey: strings.TrimSpace(apiKey),
		model:  strings.TrimSpace(model),
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type kimiSearchResponse struct {
	SearchResults []struct {
		Title    string `json:"title"`
		URL      string `json:"url"`
		SiteName string `json:"site_name"`
		Snippet  string `json:"snippet"`
		Chunks   []struct {
			Text string `json:"text"`
		} `json:"chunks"`
	} `json:"search_results"`
}

type kimiCompletionResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (g *KimiScenicSpotDescriptionGenerator) Generate(ctx context.Context, spot *models.ScenicSpot) (string, error) {
	if g == nil || g.apiKey == "" {
		return "", errors.New("kimi scenic description is not configured")
	}
	if spot == nil || strings.TrimSpace(spot.Name) == "" {
		return "", errors.New("scenic spot name is required")
	}

	queryParts := []string{strings.TrimSpace(spot.Name)}
	for _, part := range []string{spot.CityName, spot.Category, spot.Address} {
		if part = strings.TrimSpace(part); part != "" {
			queryParts = append(queryParts, part)
		}
	}
	queryParts = append(queryParts, "景点 简介 官方")
	query := strings.Join(queryParts, " ")
	searchBody, err := json.Marshal(map[string]any{
		"text_query":      query,
		"limit":           5,
		"timeout_seconds": 30,
	})
	if err != nil {
		return "", err
	}
	var search kimiSearchResponse
	if err := g.postJSON(ctx, kimiAPIBaseURL+"/tools/search_pro", searchBody, &search); err != nil {
		return "", fmt.Errorf("kimi scenic search: %w", err)
	}
	if len(search.SearchResults) == 0 {
		return "", errors.New("kimi scenic search returned no sources")
	}

	var sources strings.Builder
	for _, result := range search.SearchResults {
		title := strings.TrimSpace(result.Title)
		if title == "" {
			continue
		}
		fmt.Fprintf(&sources, "\n来源：%s（%s）\n", title, strings.TrimSpace(result.SiteName))
		if result.URL != "" {
			fmt.Fprintf(&sources, "链接：%s\n", result.URL)
		}
		if snippet := strings.TrimSpace(result.Snippet); snippet != "" {
			fmt.Fprintf(&sources, "摘要：%s\n", snippet)
		}
		for _, chunk := range result.Chunks {
			if text := strings.TrimSpace(chunk.Text); text != "" {
				fmt.Fprintf(&sources, "资料：%s\n", text)
			}
		}
	}
	if sources.Len() == 0 {
		return "", errors.New("kimi scenic search returned empty source content")
	}

	prompt := fmt.Sprintf("景点名称：%s\n城市：%s\n分类：%s\n地址：%s\n\n联网检索资料：\n%s\n\n请根据检索资料为该景点写一段通顺、简洁、适合旅游客户端展示的中文介绍。只能陈述资料明确支持的事实；资料不足以确认这是同一个景点，或没有可用介绍信息时，只输出 NO_DESCRIPTION。不要猜测，不要写营业时间、票价等易变化信息。只输出一个自然段，不超过 180 个汉字，不要标题、列表、引用标记或来源说明。", strings.TrimSpace(spot.Name), strings.TrimSpace(spot.CityName), strings.TrimSpace(spot.Category), strings.TrimSpace(spot.Address), sources.String())
	completionBody, err := json.Marshal(map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是严谨的景点资料编辑。只依据用户提供的来源资料写作，资料不支持的内容不得补充。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 1,
		// K2.6 reasoning tokens share the completion budget with its answer.
		// The prompt caps the final description, so leave ample room for reasoning.
		"max_tokens":  32768,
	})
	if err != nil {
		return "", err
	}
	var completion kimiCompletionResponse
	if err := g.postJSON(ctx, kimiAPIBaseURL+"/chat/completions", completionBody, &completion); err != nil {
		return "", fmt.Errorf("kimi scenic summary: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", errors.New("kimi scenic summary returned no choices")
	}
	choice := completion.Choices[0]
	description := strings.TrimSpace(choice.Message.Content)
	description = strings.Trim(description, "\"'“”‘’` ")
	description = strings.Join(strings.Fields(description), " ")
	if description == "" {
		return "", fmt.Errorf("kimi scenic summary returned empty content (finish_reason=%s)", choice.FinishReason)
	}
	if strings.EqualFold(description, "NO_DESCRIPTION") {
		return "", nil
	}
	if utf8.RuneCountInString(description) > maxScenicDescription {
		return "", fmt.Errorf("kimi scenic summary exceeds %d characters", maxScenicDescription)
	}
	return description, nil
}

func (g *KimiScenicSpotDescriptionGenerator) postJSON(ctx context.Context, endpoint string, body []byte, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+g.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
