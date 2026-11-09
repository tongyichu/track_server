package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/ai"
	"github.com/tongyichu/track_server/internal/models"
)

func (s *ScenicSpotCatalogService) SetAIManager(m *ai.Manager, model string) {
	s.ai = m
	s.aiModel = model
}
func scenicAIVersion(spot *models.ScenicSpot) string {
	raw, _ := json.Marshal([]any{spot.ID, spot.Version, spot.Name, spot.CityName, spot.Category, spot.Address, spot.Description})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func (s *ScenicSpotCatalogService) GenerateDescriptionPreview(ctx context.Context, spot *models.ScenicSpot) (description, taskID string, err error) {
	return s.generateScenicDescription(ctx, spot, nil)
}
func (s *ScenicSpotCatalogService) generateScenicDescription(ctx context.Context, spot *models.ScenicSpot, save func(context.Context, string) error) (description, taskID string, err error) {
	if s == nil || s.descriptionGenerator == nil {
		return "", "", errors.New("Kimi scenic description is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	ctx, run, err := s.ai.Begin(ctx, ai.Spec{Feature: "scenic_description", Provider: "moonshot", Model: s.aiModel, ObjectID: spot.ID, ObjectName: spot.Name, Reason: "景点创建或后台介绍生成", InputVersion: scenicAIVersion(spot), RuleVersion: scenicDescriptionRuleVersion, InputSummary: strings.Join([]string{spot.Name, spot.CityName, spot.Category, spot.Address}, "；") + "；联网检索后生成介绍"})
	if err != nil {
		return "", "", err
	}
	defer func() { run.Finish(err) }()
	description, err = s.descriptionGenerator.Generate(ctx, spot)
	if e := run.Check(); e != nil {
		return "", ai.TaskID(ctx), e
	}
	if err != nil {
		return "", ai.TaskID(ctx), err
	}
	adoption := "pending"
	if description == "" {
		adoption = "no_result"
	}
	if err = run.Result(map[string]string{"description": description}, adoption); err != nil {
		return "", ai.TaskID(ctx), err
	}
	if save != nil && description != "" {
		if err = run.BeforeWrite(); err != nil {
			return "", ai.TaskID(ctx), err
		}
		if err = save(ctx, description); err != nil {
			return "", ai.TaskID(ctx), err
		}
		_ = run.Adoption("applied")
	}
	return description, ai.TaskID(ctx), nil
}
func (s *ScenicSpotCatalogService) RetryAI(ctx context.Context, t *models.AITask) error {
	spot, err := s.spots.GetScenicSpot(ctx, t.ObjectID)
	if err != nil {
		return err
	}
	_, _, err = s.GenerateDescriptionPreview(ctx, spot)
	return err
}
