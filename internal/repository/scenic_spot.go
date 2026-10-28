package repository

import (
	"context"

	"github.com/tongyichu/track_server/internal/models"
)

type ScenicSpotListFilter struct {
	Query  string
	Status string
	Cursor string
	Limit  int
}

type ScenicSpotRepository interface {
	CreateScenicSpot(context.Context, *models.ScenicSpot) error
	GetScenicSpot(context.Context, string) (*models.ScenicSpot, error)
	GetScenicSpots(context.Context, []string) (map[string]*models.ScenicSpot, error)
	FindScenicSpotBySource(context.Context, string, string) (*models.ScenicSpot, error)
	ListScenicSpots(context.Context, ScenicSpotListFilter) ([]*models.ScenicSpot, int64, error)
	UpdateScenicSpot(context.Context, *models.ScenicSpot, int64) error
	GetTrackScenicSpotLink(context.Context, string) (*models.TrackScenicSpotLink, error)
	GetTrackScenicSpotLinks(context.Context, []string) (map[string]*models.TrackScenicSpotLink, error)
	SetTrackScenicSpotLink(context.Context, *models.TrackScenicSpotLink, string, bool) error
	DeleteTrackScenicSpotLink(context.Context, string, string) error
	ListTrackScenicSpotLinks(context.Context, string, string, int) ([]*models.TrackScenicSpotLink, error)
	AppendScenicSpotAudit(context.Context, *models.ScenicSpotAudit) error
	ListScenicSpotAudit(context.Context, string, string, int) ([]*models.ScenicSpotAudit, error)
	IncrementScenicSpotProviderUsage(context.Context, string, string, string, string, bool) error
	ListScenicSpotProviderUsage(context.Context, string, string) ([]*models.ScenicSpotProviderUsage, error)
	CountScenicSpotLinksBySpotIDs(context.Context, []string) (map[string]int64, error)
}
