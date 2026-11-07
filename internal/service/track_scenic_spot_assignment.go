package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type TrackMapScenicSpotInput struct {
	ProviderID     string  `json:"provider_id"`
	Name           string  `json:"name"`
	Category       string  `json:"category"`
	Address        string  `json:"address"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	ExpectedSpotID *string `json:"expected_spot_id"`
}

// AssignTrackMapScenicSpot changes only the catalog link, preserving unsaved track fields.
func (s *ScenicSpotCatalogService) AssignTrackMapScenicSpot(ctx context.Context, operator, trackID string, input TrackMapScenicSpotInput) (*models.ScenicSpot, error) {
	if input.ExpectedSpotID == nil {
		return nil, invalidScenicSpot("expected_spot_id is required")
	}
	input.ProviderID = strings.TrimSpace(input.ProviderID)
	if input.ProviderID == "" || len(input.ProviderID) > 512 || strings.TrimSpace(input.Name) == "" {
		return nil, invalidScenicSpot("请选择具有有效 POI ID 和名称的地图景点")
	}
	if math.IsNaN(input.Latitude) || math.IsInf(input.Latitude, 0) || math.IsNaN(input.Longitude) || math.IsInf(input.Longitude, 0) || input.Latitude < -90 || input.Latitude > 90 || input.Longitude < -180 || input.Longitude > 180 || input.Latitude == 0 && input.Longitude == 0 {
		return nil, invalidScenicSpot("景点坐标无效")
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if track.Status == models.TrackStatusDeleted {
		return nil, repository.ErrNotFound
	}
	currentSpotID, err := s.AssignedSpotID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if currentSpotID != *input.ExpectedSpotID {
		return nil, repository.ErrConflict
	}
	spot, err := s.spots.FindScenicSpotBySource(ctx, "amap", input.ProviderID)
	if errors.Is(err, repository.ErrNotFound) {
		now := time.Now()
		spot = &models.ScenicSpot{
			ID: newScenicID("SCP-"), Name: input.Name, Category: input.Category, Address: input.Address,
			Latitude: input.Latitude, Longitude: input.Longitude, CoordinateSystem: "GCJ02",
			Sources: []models.ScenicSpotSource{{Provider: "amap", ProviderID: input.ProviderID, Status: "active"}},
			Status:  models.ScenicSpotActive, ManualEdited: true, Version: 1,
			CreatedAt: now, UpdatedAt: now, CreatedBy: operator, UpdatedBy: operator,
		}
		if err := validateScenicSpot(spot); err != nil {
			return nil, err
		}
		if err = s.spots.CreateScenicSpot(ctx, spot); err == nil {
			s.audit(ctx, spot.ID, "create", operator, "map POI selected for track "+trackID, nil, spot)
		} else if errors.Is(err, repository.ErrAlreadyExists) {
			spot, err = s.spots.FindScenicSpotBySource(ctx, "amap", input.ProviderID)
		}
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(spot.Name) == "" {
		return nil, invalidScenicSpot("请先补齐景点目录中的名称")
	}
	if err := s.assignTrack(ctx, operator, spot.ID, trackID, "manual", input.ExpectedSpotID); err != nil {
		return nil, err
	}
	return spot, nil
}
