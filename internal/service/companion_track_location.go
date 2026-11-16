package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// Missing metadata is derived from this member's durable GPS, never a selected UI city.
func fillCompanionTrackLocation(ctx context.Context, storage repository.CompanionTrackRepository, room string, user int64, input *CreateTrackInput) error {
	if input.CityCode != nil && strings.TrimSpace(*input.CityCode) != "" && input.LocateAddr != nil && strings.TrimSpace(*input.LocateAddr) != "" {
		return nil
	}
	chunks, err := storage.ListTrackChunks(ctx, room, user, 0, 1)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	var points []CompanionRecordedPoint
	if err = json.Unmarshal([]byte(chunks[0].Data), &points); err != nil {
		return err
	}
	if len(points) == 0 {
		return nil
	}
	point, err := mapPointToGCJ02(models.TrackPoint{Latitude: points[0].Latitude, Longitude: points[0].Longitude}, "WGS84")
	if err != nil {
		return err
	}
	district := maparea.DefaultCatalog().ResolveDistrict(point.Latitude, point.Longitude)
	if district == nil {
		return nil
	}
	if input.CityCode == nil || strings.TrimSpace(*input.CityCode) == "" {
		if code, ok := config.ResolveCityCode(district.CityCode); ok {
			input.CityCode = &code
		}
	}
	if input.LocateAddr == nil || strings.TrimSpace(*input.LocateAddr) == "" {
		name := district.Name()
		if name != "" {
			input.LocateAddr = &name
		}
	}
	return nil
}

func repairCompanionTrackLocation(ctx context.Context, rooms repository.CompanionRepository, tracks repository.TrackRepository, track *models.Track) (*models.Track, error) {
	if track == nil || track.SessionID == "" || (track.CityCode != "" && track.LocateAddr != "") {
		return track, nil
	}
	origins, ok := rooms.(repository.CompanionTrackOriginRepository)
	if !ok {
		return track, nil
	}
	verified, err := origins.FindCompanionTrackIDs(ctx, []repository.CompanionTrackReference{{TrackID: track.ID, SessionID: track.SessionID, UserID: track.UserID}})
	if err != nil || !verified[track.ID] {
		return track, err
	}
	storage, ok := rooms.(repository.CompanionTrackRepository)
	if !ok {
		return track, nil
	}
	writer, ok := tracks.(repository.CompanionTrackMetadataWriter)
	if !ok {
		return track, nil
	}
	var input CreateTrackInput
	if err = fillCompanionTrackLocation(ctx, storage, track.SessionID, track.UserID, &input); err != nil {
		return track, err
	}
	city, addr := "", ""
	if input.CityCode != nil {
		city = *input.CityCode
	}
	if input.LocateAddr != nil {
		addr = *input.LocateAddr
	}
	changed, err := writer.FillMissingCompanionLocation(ctx, track, city, addr)
	if err != nil || !changed {
		return track, err
	}
	return tracks.FindByID(ctx, track.ID)
}
