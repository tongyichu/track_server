package jobs

import (
	"context"
	"log"

	"github.com/tongyichu/track_server/internal/service"
)

type ScenicPhotos struct {
	service *service.ScenicSpotCatalogService
}

func NewScenicPhotos(s *service.ScenicSpotCatalogService) *ScenicPhotos {
	return &ScenicPhotos{service: s}
}
func (j *ScenicPhotos) Name() string { return "scenic_photos" }
func (j *ScenicPhotos) Spec() string { return "@every 1m" }
func (j *ScenicPhotos) Run(ctx context.Context) error {
	result, err := j.service.SyncPhotosOnce(ctx)
	if result != nil {
		log.Printf("[scheduler] scenic_photos: scanned=%d processed=%d synced=%d failed=%d deferred=%d", result.Scanned, result.Processed, result.Synced, result.Failed, result.Deferred)
	}
	return err
}
