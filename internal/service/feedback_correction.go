package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

var ErrCorrectionOptionUnavailable = errors.New("该纠错选项已停用，请重新选择")
var correctionTrackIDPattern = regexp.MustCompile(`^NO\.[0-9A-Z]{8}$`)

func defaultCorrectionConfig() *models.FeedbackCorrectionConfig {
	return &models.FeedbackCorrectionConfig{Options: []models.FeedbackCorrectionOption{
		{ID: "scenic", Name: "景点有误", NameEN: "Wrong scenic spot", Enabled: true, SortOrder: 10},
		{ID: "district", Name: "地区有误", NameEN: "Wrong area", Enabled: true, SortOrder: 20},
		{ID: "route", Name: "线路异常", NameEN: "Route issue", Enabled: true, SortOrder: 30},
		{ID: "metrics", Name: "数据异常", NameEN: "Data issue", Enabled: true, SortOrder: 40},
		{ID: "conditions", Name: "路况变化", NameEN: "Trail conditions", Enabled: true, SortOrder: 50},
		{ID: "other", Name: "其他问题", NameEN: "Other", Enabled: true, SortOrder: 60},
	}}
}

func (s *FeedbackService) CorrectionConfig(ctx context.Context, enabledOnly bool) (*models.FeedbackCorrectionConfig, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("feedback service not configured")
	}
	config, err := s.repo.GetCorrectionConfig(ctx)
	if errors.Is(err, repository.ErrNotFound) {
		config, err = defaultCorrectionConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	options := make([]models.FeedbackCorrectionOption, 0, len(config.Options))
	for _, option := range config.Options {
		if !enabledOnly || option.Enabled {
			options = append(options, option)
		}
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].SortOrder == options[j].SortOrder {
			return options[i].ID < options[j].ID
		}
		return options[i].SortOrder < options[j].SortOrder
	})
	return &models.FeedbackCorrectionConfig{Version: config.Version, Options: options}, nil
}

func (s *FeedbackService) UpdateCorrectionConfig(ctx context.Context, input models.FeedbackCorrectionConfig) (*models.FeedbackCorrectionConfig, error) {
	current, err := s.CorrectionConfig(ctx, false)
	if err != nil {
		return nil, err
	}
	if input.Version != current.Version {
		return nil, repository.ErrAlreadyExists
	}
	if len(input.Options) > 30 {
		return nil, invalidArg("最多配置 30 个纠错选项")
	}
	existing := make(map[string]bool, len(current.Options))
	for _, option := range current.Options {
		existing[option.ID] = true
	}
	seen := make(map[string]bool, len(input.Options))
	for index := range input.Options {
		option := &input.Options[index]
		option.Name = strings.TrimSpace(option.Name)
		option.NameEN = strings.TrimSpace(option.NameEN)
		if option.Name == "" || utf8.RuneCountInString(option.Name) > 24 || utf8.RuneCountInString(option.NameEN) > 64 || strings.ContainsAny(option.Name+option.NameEN, "\r\n") {
			return nil, invalidArg("中文名称必填且不超过 24 字，英文不超过 64 字，不能换行")
		}
		if option.SortOrder < 0 || option.SortOrder > 9999 {
			return nil, invalidArg("排序应在 0 到 9999 之间")
		}
		if option.ID == "" {
			var id [8]byte
			if _, err := rand.Read(id[:]); err != nil {
				return nil, err
			}
			option.ID = "option_" + hex.EncodeToString(id[:])
		} else if !existing[option.ID] {
			return nil, invalidArg("新增选项不要指定 ID，已有选项 ID 不可修改")
		}
		if seen[option.ID] {
			return nil, invalidArg("选项不能重复")
		}
		seen[option.ID] = true
	}
	for id := range existing {
		if !seen[id] {
			return nil, invalidArg("已有选项只能停用，不能删除")
		}
	}
	input.Version++
	if err := s.repo.SaveCorrectionConfig(ctx, &input, current.Version); err != nil {
		return nil, err
	}
	return s.CorrectionConfig(ctx, false)
}

func (s *FeedbackService) prepareCorrection(ctx context.Context, in SubmitFeedbackInput) (*models.FeedbackCorrection, string, error) {
	trackID := strings.TrimSpace(in.TrackID)
	optionID := strings.TrimSpace(in.CorrectionOptionID)
	if !correctionTrackIDPattern.MatchString(trackID) || optionID == "" {
		return nil, "", invalidArg("track_id and correction_option_id are required")
	}
	notes := strings.TrimSpace(in.Content)
	if utf8.RuneCountInString(notes) > 500 {
		return nil, "", invalidArg("correction notes must be <= 500 characters")
	}
	config, err := s.CorrectionConfig(ctx, true)
	if err != nil {
		return nil, "", err
	}
	var selected *models.FeedbackCorrectionOption
	for index := range config.Options {
		if config.Options[index].ID == optionID {
			selected = &config.Options[index]
			break
		}
	}
	if selected == nil {
		return nil, "", ErrCorrectionOptionUnavailable
	}
	if s.correctionTracks == nil {
		return nil, "", errors.New("correction track repository not configured")
	}
	track, err := s.correctionTracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, "", err
	}
	if track.Status == models.TrackStatusDeleted || track.IsRunning || strings.TrimSpace(track.RawTrackURL) == "" {
		return nil, "", repository.ErrNotFound
	}
	if track.UserID != in.UserID && track.Status != models.TrackStatusNormal {
		return nil, "", repository.ErrForbidden
	}
	correction := &models.FeedbackCorrection{
		TrackID: trackID, TrackTitle: track.Title, OptionID: selected.ID,
		OptionName: selected.Name, OptionNameEN: selected.NameEN, Notes: notes,
	}
	title := trimMax(strings.NewReplacer("\n", " ", "\r", " ").Replace(track.Title), 120)
	content := fmt.Sprintf("轨迹纠错：%s\n轨迹：%s\nID：%s", selected.Name, title, trackID)
	if strings.HasPrefix(strings.ToLower(in.Language), "en") {
		name := selected.NameEN
		if name == "" {
			name = selected.Name
		}
		content = fmt.Sprintf("Route correction: %s\nRoute: %s\nID: %s", name, title, trackID)
	}
	if notes != "" {
		content += "\n\n" + notes
	}
	return correction, content, nil
}
