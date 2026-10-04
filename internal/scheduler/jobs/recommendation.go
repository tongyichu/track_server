package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/tongyichu/track_server/internal/service"
)

type RecommendationProfile struct {
	service *service.RecommendationService
	spec    string
}

func NewRecommendationProfile(recommendationSvc *service.RecommendationService, spec string) *RecommendationProfile {
	if strings.TrimSpace(spec) == "" {
		spec = "0 5 * * *"
	}
	return &RecommendationProfile{service: recommendationSvc, spec: spec}
}
func (j *RecommendationProfile) Name() string { return "recommendation_profile" }
func (j *RecommendationProfile) Spec() string { return j.spec }
func (j *RecommendationProfile) Run(ctx context.Context) error {
	if j.service == nil {
		return fmt.Errorf("recommendation_profile: service is nil")
	}
	count, err := j.service.RebuildUserProfiles(ctx)
	log.Printf("[scheduler] recommendation_profile: profiles=%d", count)
	return err
}

type RecommendationItemStats struct {
	service *service.RecommendationService
	spec    string
}

func NewRecommendationItemStats(recommendationSvc *service.RecommendationService, spec string) *RecommendationItemStats {
	if strings.TrimSpace(spec) == "" {
		spec = "30 5 * * *"
	}
	return &RecommendationItemStats{service: recommendationSvc, spec: spec}
}
func (j *RecommendationItemStats) Name() string { return "recommendation_item_stats" }
func (j *RecommendationItemStats) Spec() string { return j.spec }
func (j *RecommendationItemStats) Run(ctx context.Context) error {
	if j.service == nil {
		return fmt.Errorf("recommendation_item_stats: service is nil")
	}
	count, err := j.service.RebuildItemStats(ctx)
	log.Printf("[scheduler] recommendation_item_stats: items=%d", count)
	return err
}

type RecommendationSessionCleanup struct {
	service *service.RecommendationService
	spec    string
}

func NewRecommendationSessionCleanup(recommendationSvc *service.RecommendationService, spec string) *RecommendationSessionCleanup {
	if strings.TrimSpace(spec) == "" {
		spec = "@every 20m"
	}
	return &RecommendationSessionCleanup{service: recommendationSvc, spec: spec}
}
func (j *RecommendationSessionCleanup) Name() string { return "recommendation_session_cleanup" }
func (j *RecommendationSessionCleanup) Spec() string { return j.spec }
func (j *RecommendationSessionCleanup) Run(ctx context.Context) error {
	if j.service == nil {
		return fmt.Errorf("recommendation_session_cleanup: service is nil")
	}
	count, err := j.service.CleanupFeedSessions(ctx)
	log.Printf("[scheduler] recommendation_session_cleanup: deleted=%d", count)
	return err
}
