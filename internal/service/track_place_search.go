package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func parseAMapTrackPlace(a amapPlace, expires time.Time) (models.TrackPlace, bool) {
	id, name := string(a.ID), strings.TrimSpace(string(a.Name))
	category := string(a.TypeCode)
	if category == "" {
		category = string(a.Type)
	}
	if id == "" || len(id) > 128 || name == "" || len([]rune(name)) > 128 || !amapScenicType(category) {
		return models.TrackPlace{}, false
	}
	parts := strings.Split(string(a.Location), ",")
	if len(parts) != 2 {
		return models.TrackPlace{}, false
	}
	lng, e1 := strconv.ParseFloat(parts[0], 64)
	lat, e2 := strconv.ParseFloat(parts[1], 64)
	if e1 != nil || e2 != nil || !validMapLatLng(lat, lng) {
		return models.TrackPlace{}, false
	}
	return models.TrackPlace{ID: placeID("amap", id), Provider: "amap", ProviderID: id, Name: name, Category: category, ParentID: string(a.Parent), DistrictCode: string(a.Adcode), Latitude: lat, Longitude: lng, CoordinateSystem: "GCJ02", ExpiresAt: expires}, true
}

func (s *TrackPlaceService) enclosedPlaces(ctx context.Context, index *models.TrackGeoIndex, ring *trackPlaceRing) ([]*models.TrackPlace, error) {
	result := map[string]*models.TrackPlace{}
	for page := 1; page <= 4; page++ {
		key := fmt.Sprintf("ring:1:amap:%s:%d", trackPlaceSourceVersion(index), page)
		values, err := s.cachedPlaceLookup(ctx, "amap", key, func() ([]placeEvidence, error) { return s.amapRingPage(ctx, ring, page) })
		if err != nil {
			return nil, err
		}
		count := -1
		for _, value := range values {
			if value.Place.ID == "" {
				count = value.SearchCount
				continue
			}
			p := value.Place
			if ring.contains(p.Latitude, p.Longitude) {
				result[p.ID] = &p
			}
		}
		// A truncated search cannot establish that one candidate is unique.
		if count < 0 {
			return nil, errors.New("place_provider_invalid_search_count")
		}
		if count > 100 {
			return nil, nil
		}
		if page*25 >= count {
			break
		}
	}
	out := make([]*models.TrackPlace, 0, len(result))
	for _, p := range result {
		out = append(out, p)
	}
	return out, nil
}

func (s *TrackPlaceService) amapRingPage(ctx context.Context, ring *trackPlaceRing, page int) ([]placeEvidence, error) {
	q := url.Values{"key": {s.cfg.AMapKey}, "polygon": {fmt.Sprintf("%.6f,%.6f|%.6f,%.6f", ring.minLng, ring.maxLat, ring.maxLng, ring.minLat)}, "types": {"110000"}, "offset": {"25"}, "page": {strconv.Itoa(page)}, "extensions": {"base"}, "output": {"json"}}
	req, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/v3/place/polygon?"+q.Encode(), nil)
	var response struct {
		Status   string        `json:"status"`
		InfoCode string        `json:"infocode"`
		Count    amapPlaceText `json:"count"`
		POIs     []amapPlace   `json:"pois"`
	}
	if err := s.doProviderRequest(ctx, req, &response); err != nil {
		return nil, err
	}
	if response.Status != "1" {
		return nil, fmt.Errorf("amap_error_%s", safePlaceCode(response.InfoCode))
	}
	count, err := strconv.Atoi(string(response.Count))
	if err != nil || count < 0 {
		return nil, errors.New("place_provider_invalid_search_count")
	}
	out := []placeEvidence{{SearchCount: count}}
	if count > 100 {
		return out, nil
	}
	for _, poi := range response.POIs {
		if p, ok := parseAMapTrackPlace(poi, time.Now().Add(s.providerTTL("amap"))); ok {
			out = append(out, placeEvidence{Place: p})
		}
	}
	return out, nil
}
