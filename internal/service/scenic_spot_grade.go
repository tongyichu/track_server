package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

var scenicGradePattern = regexp.MustCompile(`^(?:国家|中国)?(?:([1-5])A|(A{1,5}))(?:级)?(?:旅游)?(?:景区|风景区|旅游区)?$`)
var scenicGradeSeparators = regexp.MustCompile(`[,，、;；|/]+`)

// Only explicit A-grade labels qualify. Ratings, stars and descriptions do not.
func normalizeScenicGrade(raw string) string {
	raw = strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	raw = strings.NewReplacer("Ａ", "A", "１", "1", "２", "2", "３", "3", "４", "4", "５", "5").Replace(raw)
	match := scenicGradePattern.FindStringSubmatch(raw)
	if match == nil {
		return ""
	}
	if match[1] != "" {
		return match[1] + "A"
	}
	return string(rune('0'+len(match[2]))) + "A"
}

func scenicSpotGradeRank(grade string) int {
	if normalized := normalizeScenicGrade(grade); normalized != "" {
		return int(normalized[0] - '0')
	}
	return 0
}

func scenicGradeFromKeytag(raw string) string {
	grade := ""
	for _, label := range scenicGradeSeparators.Split(raw, -1) {
		candidate := normalizeScenicGrade(label)
		if candidate == "" {
			continue
		}
		if grade != "" && grade != candidate {
			return ""
		}
		grade = candidate
	}
	return grade
}

func scenicGradeAMapSource(spot *models.ScenicSpot) string {
	for _, source := range spot.Sources {
		if source.Provider == "amap" && source.Status == "active" && source.ProviderID != "" {
			return "amap:" + source.ProviderID
		}
	}
	return ""
}

func resetCollectedScenicGrade(spot *models.ScenicSpot) {
	if spot.ScenicGradeSource == "manual" {
		return
	}
	spot.ScenicGrade, spot.ScenicGradeSource, spot.ScenicGradeStatus = "", "", ""
	spot.ScenicGradeCheckedAt = nil
}

// Fetch by exact POI ID; do not inherit a parent attraction's grade or search by name.
func (s *TrackPlaceService) scenicSpotAMapGrade(ctx context.Context, spotID, providerID string) (string, error) {
	if !s.Enabled() || s.cfg.AMapKey == "" {
		return "", errors.New("amap grade provider unavailable")
	}
	if !s.reserveScenicCall(ctx, "amap") {
		return "", errors.New("amap grade quota unavailable")
	}
	q := url.Values{"key": {s.cfg.AMapKey}, "id": {providerID}, "show_fields": {"business"}, "output": {"json"}}
	req, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/v5/place/detail?"+q.Encode(), nil)
	var result struct {
		Status string `json:"status"`
		Pois   []struct {
			ID       string          `json:"id"`
			Business json.RawMessage `json:"business"`
		} `json:"pois"`
	}
	err := s.doProviderRequest(ctx, req, &result)
	succeeded := err == nil && result.Status == "1" && len(result.Pois) == 1 && result.Pois[0].ID == providerID
	s.recordScenicProviderCall(ctx, spotID, "amap", "grade", succeeded)
	if !succeeded {
		return "", errors.New("amap grade lookup failed")
	}
	var business struct {
		Keytag amapPlaceText `json:"keytag"`
	}
	raw := strings.TrimSpace(string(result.Pois[0].Business))
	if raw == "" || raw == "null" || raw == "[]" {
		return "", nil
	}
	if json.Unmarshal(result.Pois[0].Business, &business) != nil {
		return "", errors.New("amap grade payload invalid")
	}
	return scenicGradeFromKeytag(string(business.Keytag)), nil
}

func scenicGradeNeedsSync(spot *models.ScenicSpot) bool {
	return spot.Status == models.ScenicSpotActive && spot.ScenicGradeSource != "manual" && (spot.ScenicGradeStatus == "" || spot.ScenicGradeStatus == "failed")
}

// Collect before an admin write so its response version remains usable for photo uploads.
func (s *ScenicSpotCatalogService) collectScenicGrade(ctx context.Context, spot *models.ScenicSpot) error {
	if !scenicGradeNeedsSync(spot) {
		return nil
	}
	source := scenicGradeAMapSource(spot)
	grade := ""
	var fetchErr error
	if source != "" {
		if s.provider == nil {
			fetchErr = errors.New("grade provider unavailable")
		} else {
			fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			grade, fetchErr = s.provider.scenicSpotAMapGrade(fetchCtx, spot.ID, strings.TrimPrefix(source, "amap:"))
		}
	}
	now := time.Now()
	spot.ScenicGradeCheckedAt = &now
	spot.ScenicGradeSource = source
	spot.ScenicGradeStatus = "unavailable"
	if fetchErr != nil {
		spot.ScenicGradeStatus = "failed"
	} else {
		spot.ScenicGrade = grade
		if grade != "" {
			spot.ScenicGradeStatus = "synced"
		}
	}
	return fetchErr
}

// Grade sync is independent of text/photo sync, so older synced records are eligible.
// A missing grade is a persisted negative result; only failed requests are retried.
func (s *ScenicSpotCatalogService) syncScenicGrade(ctx context.Context, spot *models.ScenicSpot, operator string) (bool, error) {
	if !scenicGradeNeedsSync(spot) {
		return false, nil
	}
	candidate := *spot
	fetchErr := s.collectScenicGrade(ctx, &candidate)
	// Reload after network IO to preserve concurrent description and admin edits.
	latest, err := s.spots.GetScenicSpot(ctx, spot.ID)
	if err != nil {
		return false, err
	}
	if !scenicGradeNeedsSync(latest) || scenicGradeAMapSource(latest) != scenicGradeAMapSource(spot) {
		*spot = *latest
		return false, nil
	}
	latest.ScenicGrade = candidate.ScenicGrade
	latest.ScenicGradeSource = candidate.ScenicGradeSource
	latest.ScenicGradeStatus = candidate.ScenicGradeStatus
	latest.ScenicGradeCheckedAt = candidate.ScenicGradeCheckedAt
	*spot = *latest
	if err := s.persistProviderContentSync(ctx, spot, operator, "scenic grade "+spot.ScenicGradeStatus); err != nil {
		return false, err
	}
	return spot.ScenicGrade != "", fetchErr
}

func (s *ScenicSpotCatalogService) autoCollectScenicGrade(spotID string) {
	if !s.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spot, err := s.spots.GetScenicSpot(ctx, spotID)
	if err == nil {
		_, err = s.syncScenicGrade(ctx, spot, "system")
	}
	if err != nil {
		log.Printf("auto scenic grade sync failed: spot=%s err=%v", spotID, err)
	}
}
