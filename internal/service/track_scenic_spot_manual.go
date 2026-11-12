package service

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// Manual entry has no invented POI identity or coordinates and performs no
// provider/AI requests. Catalog creation and assignment follow the map flow's
// existing CAS and audit boundaries.
func (s *ScenicSpotCatalogService) assignTrackManualScenicSpot(ctx context.Context, operator, trackID string, input TrackMapScenicSpotInput) (*models.ScenicSpot, error) {
	spotID, name := strings.TrimSpace(input.ScenicSpotID), strings.TrimSpace(input.ManualName)
	if (spotID == "") == (name == "") || input.ProviderID != "" || input.Name != "" || input.Category != "" || input.Address != "" || input.Latitude != 0 || input.Longitude != 0 {
		return nil, invalidScenicSpot("请选择已有景点，或填写一个新景点名称")
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if track.Status == models.TrackStatusDeleted {
		return nil, repository.ErrNotFound
	}
	current, err := s.AssignedSpotID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if current != *input.ExpectedSpotID {
		return nil, repository.ErrConflict
	}
	var spot *models.ScenicSpot
	if spotID != "" {
		spot, err = s.spots.GetScenicSpot(ctx, spotID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(spot.Name) == "" {
			return nil, invalidScenicSpot("请先补齐景点目录中的名称")
		}
	} else {
		if strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return nil, invalidScenicSpot("景点名称不能包含控制字符")
		}
		now := time.Now()
		spot = &models.ScenicSpot{
			ID: newScenicID("SCP-"), Name: name, CityCode: config.CanonicalCityCode(track.CityCode),
			CityName: config.CityNameByCode(config.CanonicalCityCode(track.CityCode)),
			Status:   models.ScenicSpotActive, ManualEdited: true, Version: 1,
			CreatedAt: now, UpdatedAt: now, CreatedBy: operator, UpdatedBy: operator,
		}
		if err := validateScenicSpot(spot); err != nil {
			return nil, err
		}
		if err := s.spots.CreateScenicSpot(ctx, spot); err != nil {
			return nil, err
		}
		s.audit(ctx, spot.ID, "create", operator, "manual name for track "+trackID, nil, spot)
	}
	if err := s.assignTrack(ctx, operator, spot.ID, trackID, "manual", input.ExpectedSpotID); err != nil {
		return nil, err
	}
	return spot, nil
}
