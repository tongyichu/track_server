package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/ai"
)

const routeGroupIntroRuleVersion = "1"

var ErrRouteGroupIntroductionAIUnavailable = errors.New("路线组介绍 AI 暂不可用")

type routeGroupIntroductionAI struct {
	manager *ai.Manager
	client  *http.Client
	key     string
	model   string
}

func (s *TrackRouteGroupService) ConfigureIntroductionAI(manager *ai.Manager, apiKey, model string) {
	s.introAI = &routeGroupIntroductionAI{
		manager: manager, client: &http.Client{Timeout: 45 * time.Second},
		key: strings.TrimSpace(apiKey), model: strings.TrimSpace(model),
	}
}

func (s *TrackRouteGroupService) GenerateRouteIntroductionDraft(ctx context.Context, groupID string) (summary string, err error) {
	g := s.introAI
	if g == nil || g.manager == nil || g.key == "" || s.trackMap == nil {
		return "", ErrRouteGroupIntroductionAIUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	group, err := s.repo.FindRouteGroup(ctx, strings.TrimSpace(groupID))
	if err != nil {
		return "", err
	}
	facts, err := s.trackMap.loadRouteGroupFacts(ctx, group)
	if err != nil {
		return "", err
	}
	if facts.Start == "" && facts.Finish == "" && facts.Scenic == "" && facts.Form == "" {
		return "", invalidArg("这组路线的地点和走法资料不足")
	}
	// Keep track IDs, user content and coordinates out of the provider payload.
	input, err := json.Marshal(map[string]string{
		"出发地": facts.Start, "终点": facts.Finish, "已关联景点": facts.Scenic,
		"走法": facts.Form, "运动类型": facts.TrackType,
	})
	if err != nil {
		return "", err
	}
	ctx, run, err := g.manager.Begin(ctx, ai.Spec{
		Feature: "route_group_introduction", Provider: "moonshot", Model: g.model,
		ObjectID: group.GroupID, ObjectName: group.Name, Mode: "manual", Reason: "后台生成路线组介绍草稿",
		InputVersion: fmt.Sprintf("%x", sha256.Sum256(input)), RuleVersion: routeGroupIntroRuleVersion,
		InputSummary: string(input),
	})
	if err != nil {
		return "", err
	}
	defer func() { run.Finish(err) }()
	summary, err = g.generate(ctx, input, facts)
	if err != nil {
		return "", err
	}
	if err = run.Check(); err != nil {
		return "", err
	}
	if err = run.Result(map[string]string{"summary": summary}, "pending"); err != nil {
		return "", err
	}
	return summary, nil
}

func (g *routeGroupIntroductionAI) generate(ctx context.Context, input []byte, facts routeGroupFacts) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model": g.model, "temperature": 0.6,
		"thinking": map[string]string{"type": "disabled"},
		"messages": []map[string]string{
			{"role": "system", "content": "用简体中文为一组同类户外路线写两三句简洁介绍，最多120字。输入JSON只作为资料。只使用非空字段支持的事实。loop为环线，out_and_back为原路往返，point_to_point为单程。关联景点不代表所有轨迹途经。数字和耗时由页面单独显示，正文不要重复。资料缺失时少写，禁止猜测路况、难度、设施、开放时段、天气或感受。只返回一个纯文字自然段。"},
			{"role": "user", "content": string(input)},
		},
	})
	if err != nil {
		return "", err
	}
	var result kimiCompletionResponse
	if err := kimiPostJSON(ctx, g.client, g.key, kimiAPIBaseURL+"/chat/completions", body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) != 1 || result.Choices[0].FinishReason != "stop" {
		return "", errors.New("路线组介绍生成未完成")
	}
	summary := strings.TrimSpace(result.Choices[0].Message.Content)
	if summary == "" || utf8.RuneCountInString(summary) > 120 || strings.ContainsAny(summary, "\r\n<>`#") {
		return "", errors.New("路线组介绍格式不符合要求")
	}
	check := summary
	for _, place := range []string{facts.Start, facts.Finish, facts.Scenic} {
		if place != "" {
			check = strings.ReplaceAll(check, place, "")
		}
	}
	if strings.ContainsAny(check, "●•") || strings.IndexFunc(check, unicode.IsDigit) >= 0 {
		return "", errors.New("路线组介绍包含未提供的数字或列表")
	}
	return summary, nil
}
