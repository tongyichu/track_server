package service

import (
	"context"
	"slices"
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	scenicWholeSpotScope = "spot"
	scenicMapBatchSize   = 100
	// The catalog may contain WGS84 or BD09. Fetch a margin before exact GCJ02 filtering.
	scenicMapCoordinateMargin = 0.02
)

func catalogScenicMapMatch(spot *models.ScenicSpot) *departureScenicMatch {
	match := &departureScenicMatch{
		id: spot.ID, name: spot.Name, cityCode: config.CanonicalCityCode(spot.CityCode),
		catalog: true, rank: departureScenicAutomatic,
	}
	if validProviderCoordinate(spot.Latitude, spot.Longitude) {
		point, err := mapPointToGCJ02(models.TrackPoint{Latitude: spot.Latitude, Longitude: spot.Longitude}, spot.CoordinateSystem)
		if err == nil && validMapLatLng(point.Latitude, point.Longitude) {
			match.point = &models.TrackMapPoint{Latitude: point.Latitude, Longitude: point.Longitude}
		}
	}
	return match
}

func departurePointVisible(point models.TrackMapPoint, filter models.TrackMapQueryFilter) bool {
	if b := filter.BBox; b != nil &&
		(point.Latitude < b.MinLatitude || point.Latitude > b.MaxLatitude ||
			point.Longitude < b.MinLongitude || point.Longitude > b.MaxLongitude) {
		return false
	}
	return filter.Center == nil ||
		haversineDistance(filter.Center.Latitude, filter.Center.Longitude, point.Latitude, point.Longitude) <= float64(filter.RadiusM)
}

func (s *TrackMapService) visitVisibleScenicPins(ctx context.Context, filter models.TrackMapQueryFilter, visit func(mapDeparture)) error {
	if s.trackSvc == nil || !s.trackSvc.scenicCatalog.Enabled() || filter.BBox == nil {
		return nil
	}
	bounds := *filter.BBox
	if bounds.MinLatitude > bounds.MaxLatitude || bounds.MinLongitude > bounds.MaxLongitude {
		return nil
	}
	bounds.MinLatitude -= scenicMapCoordinateMargin
	bounds.MaxLatitude += scenicMapCoordinateMargin
	bounds.MinLongitude -= scenicMapCoordinateMargin
	bounds.MaxLongitude += scenicMapCoordinateMargin
	catalog := s.trackSvc.scenicCatalog.spots
	spots, err := catalog.ListScenicMapSpots(ctx, bounds)
	if err != nil {
		return err
	}
	matches := make(map[string]*departureScenicMatch, len(spots))
	spotIDs := make([]string, 0, len(spots))
	for _, spot := range spots {
		if !scenicSpotClientVisible(spot) || strings.TrimSpace(spot.Name) == "" {
			continue
		}
		match := catalogScenicMapMatch(spot)
		if match.point != nil && departurePointVisible(*match.point, filter) {
			matches[spot.ID] = match
			spotIDs = append(spotIDs, spot.ID)
		}
	}
	for start := 0; start < len(spotIDs); start += scenicMapBatchSize {
		spotBatch := spotIDs[start:min(start+scenicMapBatchSize, len(spotIDs))]
		spotByTrack := make(map[string]string)
		var trackIDs []string
		err := catalog.VisitScenicMapLinks(ctx, spotBatch, func(trackID, spotID string) {
			spotByTrack[trackID] = spotID
			trackIDs = append(trackIDs, trackID)
		})
		if err != nil {
			return err
		}
		for start := 0; start < len(trackIDs); start += scenicMapBatchSize {
			batch := trackIDs[start:min(start+scenicMapBatchSize, len(trackIDs))]
			// The pin is already in view. Its tracks may start anywhere around the spot.
			err := s.visitMapDepartures(ctx, models.TrackMapQueryFilter{TrackType: filter.TrackType, TrackIDs: batch}, "", "", func(d mapDeparture) {
				d.scenic = matches[spotByTrack[d.TrackID]]
				visit(d)
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *TrackMapService) listWholeScenicSpotTracks(ctx context.Context, userID int64, spotID, trackType, cursor string, limit int) (*models.TrackSummaryPage, error) {
	if s.trackSvc == nil || !s.trackSvc.scenicCatalog.Enabled() {
		return nil, repository.ErrNotFound
	}
	catalog := s.trackSvc.scenicCatalog.spots
	spot, err := catalog.GetScenicSpot(ctx, spotID)
	if err != nil {
		return nil, err
	}
	if !scenicSpotClientVisible(spot) {
		return nil, repository.ErrNotFound
	}
	trackTypeCode := trackMapTypeFilter(trackType)
	limit = normalizeTrackPageLimit(limit)
	tracks := make([]*models.Track, 0, limit+1)
	for len(tracks) <= limit {
		links, err := catalog.ListTrackScenicSpotLinks(ctx, spotID, cursor, scenicMapBatchSize)
		if err != nil {
			return nil, err
		}
		if len(links) == 0 {
			break
		}
		trackIDs := make([]string, 0, len(links))
		for _, link := range links {
			trackIDs = append(trackIDs, link.TrackID)
		}
		cursor = trackIDs[len(trackIDs)-1]
		var visibleIDs []string
		err = s.visitMapDepartures(ctx, models.TrackMapQueryFilter{TrackType: trackTypeCode, TrackIDs: trackIDs}, "", "", func(d mapDeparture) {
			visibleIDs = append(visibleIDs, d.TrackID)
		})
		if err != nil {
			return nil, err
		}
		slices.Sort(visibleIDs)
		if len(visibleIDs) > 0 {
			byID, err := s.tracks.FindByIDs(ctx, visibleIDs)
			if err != nil {
				return nil, err
			}
			for _, id := range visibleIDs {
				if track := byID[id]; publicMapTrack(track) && track.RawTrackURL != "" {
					tracks = append(tracks, track)
					if len(tracks) > limit {
						break
					}
				}
			}
		}
		if len(links) < scenicMapBatchSize {
			break
		}
	}
	return s.scenicTracksPage(ctx, userID, tracks, limit)
}
