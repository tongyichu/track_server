package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/tongyichu/track_server/internal/models"
)

var errPlaceBudget = errors.New("place_daily_budget_exhausted")
var errPlaceBusy = errors.New("place_lookup_busy")

type placeEvidence struct {
	Place       models.TrackPlace `json:"place"`
	Inside      bool              `json:"inside"`
	SearchCount int               `json:"search_count,omitempty"`
}
type amapPlaceText string

func (t *amapPlaceText) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, []byte("null")) {
		*t = ""
		return nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err == nil {
		*t = amapPlaceText(v)
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	*t = amapPlaceText(strings.Join(values, ", "))
	return nil
}

type amapPlace struct {
	ID       amapPlaceText `json:"id"`
	Name     amapPlaceText `json:"name"`
	Type     amapPlaceText `json:"type"`
	TypeCode amapPlaceText `json:"typecode"`
	Location amapPlaceText `json:"location"`
	Distance amapPlaceText `json:"distance"`
	Adcode   amapPlaceText `json:"adcode"`
	Parent   amapPlaceText `json:"parent"`
}

func placeID(provider, id string) string {
	return "place-" + provider + "-" + fmt.Sprintf("%x", sha256.Sum256([]byte(id)))[:24]
}

// Endpoint URLs are fixed. Never return transport errors containing AMap's key
// query parameter, nor raw provider response bodies, to API clients or logs.
func (s *TrackPlaceService) providerLookup(ctx context.Context, provider string, p models.TrackPoint) ([]placeEvidence, error) {
	return s.providerLookupWithOptions(ctx, provider, p, false)
}

func (s *TrackPlaceService) providerLookupWithOptions(ctx context.Context, provider string, p models.TrackPoint, forceRefresh bool) ([]placeEvidence, error) {
	if provider == "google" {
		p = mapGCJ02ToWGS84(p)
	}
	key := fmt.Sprintf("request:%d:%s:%.6f:%.6f", trackPlaceEvidenceVersion, provider, p.Latitude, p.Longitude)
	return s.cachedPlaceLookupWithOptions(ctx, provider, key, forceRefresh, func() ([]placeEvidence, error) {
		if provider == "amap" {
			return s.amapLookup(ctx, p)
		}
		return s.googleLookup(ctx, p)
	})
}

func (s *TrackPlaceService) cachedPlaceLookup(ctx context.Context, provider, key string, lookup func() ([]placeEvidence, error)) ([]placeEvidence, error) {
	return s.cachedPlaceLookupWithOptions(ctx, provider, key, false, lookup)
}

func (s *TrackPlaceService) cachedPlaceLookupWithOptions(ctx context.Context, provider, key string, forceRefresh bool, lookup func() ([]placeEvidence, error)) ([]placeEvidence, error) {
	var cached []placeEvidence
	if !forceRefresh {
		if ok, err := s.loadFresh(ctx, key, &cached); err != nil {
			return nil, err
		} else if ok {
			return cached, nil
		}
	}
	if s.runCalls >= 10 {
		return nil, errPlaceBusy
	}
	owner := s.owner()
	now := time.Now()
	leased, err := s.store.LeasePlaceRecord(ctx, key, owner, now, now.Add(time.Minute))
	if err != nil {
		return nil, err
	}
	if !leased {
		return nil, errPlaceBusy
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if record, e := s.store.GetPlaceRecord(releaseCtx, key); e == nil && record.LeaseOwner == owner {
			_, _ = s.store.SavePlaceRecord(releaseCtx, record, owner)
		}
	}()
	// A second worker may have completed between the first cache read and lease.
	if !forceRefresh {
		if ok, err := s.loadFresh(ctx, key, &cached); err != nil {
			return nil, err
		} else if ok {
			if record, e := s.store.GetPlaceRecord(ctx, key); e == nil {
				_, _ = s.store.SavePlaceRecord(ctx, record, owner)
			}
			return cached, nil
		}
	}
	var paused string
	if pause, e := s.loadFresh(ctx, "pause:"+provider, &paused); e != nil {
		return nil, e
	} else if pause {
		return nil, errors.New(paused)
	}
	budget := s.cfg.AMapDailyBudget
	if provider == "google" {
		budget = s.cfg.GoogleDailyBudget
	}
	allowed, err := s.store.ReservePlaceAPICall(ctx, provider, time.Now().In(placeQuotaZone).Format("2006-01-02"), budget)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errPlaceBudget
	}
	if wait := 500*time.Millisecond - time.Since(s.lastCall); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	s.lastCall = time.Now()
	s.runCalls++
	values, err := lookup()
	if err != nil {
		code := err.Error()
		if strings.HasPrefix(code, "amap_error_100") || code == "place_provider_permission_denied" || code == "place_provider_rate_limited" {
			_ = s.save(ctx, "pause:"+provider, code, now.Add(time.Hour), "")
		}
		return nil, err
	}
	ttl := s.providerTTL(provider)
	empty := len(values) == 0 || (len(values) == 1 && values[0].Place.ID == "" && values[0].SearchCount == 0)
	if empty && ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	if err := s.save(ctx, key, values, now.Add(ttl), owner); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *TrackPlaceService) doProviderRequest(ctx context.Context, req *http.Request, target interface{}) error {
	resp, err := s.client.Do(req.WithContext(ctx))
	if err != nil {
		return errors.New("place_provider_transport_error")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return errors.New("place_provider_rate_limited")
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("place_provider_permission_denied")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("place_provider_http_%d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(target); err != nil {
		return errors.New("place_provider_invalid_response")
	}
	return nil
}
func (s *TrackPlaceService) amapLookup(ctx context.Context, p models.TrackPoint) ([]placeEvidence, error) {
	q := url.Values{"key": {s.cfg.AMapKey}, "location": {fmt.Sprintf("%.6f,%.6f", p.Longitude, p.Latitude)}, "extensions": {"all"}, "radius": {"300"}, "output": {"json"}}
	req, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/v3/geocode/regeo?"+q.Encode(), nil)
	var result struct {
		Status    string `json:"status"`
		InfoCode  string `json:"infocode"`
		Regeocode struct {
			AOIs []amapPlace `json:"aois"`
			POIs []amapPlace `json:"pois"`
		} `json:"regeocode"`
	}
	if err := s.doProviderRequest(ctx, req, &result); err != nil {
		return nil, err
	}
	if result.Status != "1" {
		return nil, fmt.Errorf("amap_error_%s", safePlaceCode(result.InfoCode))
	}
	out := []placeEvidence{}
	seen := map[string]bool{}
	add := func(a amapPlace, inside bool) {
		p, ok := parseAMapTrackPlace(a, time.Now().Add(s.providerTTL("amap")))
		if !ok || seen[p.ProviderID] {
			return
		}
		seen[p.ProviderID] = true
		out = append(out, placeEvidence{Place: p, Inside: inside})
	}
	for _, a := range result.Regeocode.AOIs {
		add(a, string(a.Distance) == "0")
	}
	for _, a := range result.Regeocode.POIs {
		add(a, false)
	}
	return out, nil
}
func amapScenicType(category string) bool {
	return strings.HasPrefix(category, "11") || strings.Contains(category, "风景名胜") || strings.Contains(category, "公园")
}

func (s *TrackPlaceService) googleLookup(ctx context.Context, p models.TrackPoint) ([]placeEvidence, error) {
	body, _ := json.Marshal(map[string]interface{}{"includedTypes": []string{"tourist_attraction", "park", "national_park", "museum"}, "maxResultCount": 5, "rankPreference": "DISTANCE", "languageCode": "en", "locationRestriction": map[string]interface{}{"circle": map[string]interface{}{"center": map[string]float64{"latitude": p.Latitude, "longitude": p.Longitude}, "radius": 200}}})
	req, _ := http.NewRequest(http.MethodPost, "https://places.googleapis.com/v1/places:searchNearby", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", s.cfg.GoogleKey)
	req.Header.Set("X-Goog-FieldMask", "places.id,places.displayName,places.primaryType,places.location,places.containingPlaces")
	var result struct {
		Places []struct {
			ID          string `json:"id"`
			DisplayName struct {
				Text string `json:"text"`
			} `json:"displayName"`
			PrimaryType string `json:"primaryType"`
			Location    struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
			ContainingPlaces []struct {
				ID string `json:"id"`
			} `json:"containingPlaces"`
		} `json:"places"`
	}
	if err := s.doProviderRequest(ctx, req, &result); err != nil {
		return nil, err
	}
	out := []placeEvidence{}
	for _, p := range result.Places {
		if p.ID == "" || len(p.ID) > 256 || p.DisplayName.Text == "" || !validMapLatLng(p.Location.Latitude, p.Location.Longitude) {
			continue
		}
		parent := ""
		if len(p.ContainingPlaces) > 0 {
			parent = p.ContainingPlaces[0].ID
		}
		out = append(out, placeEvidence{Place: models.TrackPlace{ID: placeID("google", p.ID), Provider: "google", ProviderID: p.ID, Name: p.DisplayName.Text, Category: p.PrimaryType, ParentID: parent, Latitude: p.Location.Latitude, Longitude: p.Location.Longitude, CoordinateSystem: "WGS84", ExpiresAt: time.Now().Add(s.providerTTL("google"))}, Inside: false})
	}
	return out, nil
}
func safePlaceCode(code string) string {
	if len(code) != 5 {
		return "unknown"
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return "unknown"
		}
	}
	return code
}

// ScenicSpotDetails reads provider content only for a track detail response. Provider
// descriptions, photo references and media URLs are never persisted.
func (s *TrackPlaceService) ScenicSpotDetails(ctx context.Context, spot *models.TrackPrimaryScenicSpot) {
	if spot == nil {
		return
	}
	if len(spot.Sources) > 0 {
		for _, source := range spot.Sources {
			if source.Status != "active" || source.ProviderID == "" {
				continue
			}
			if spot.Address != "" && spot.Description != "" && len(spot.Photos) >= 3 {
				break
			}
			candidate := &models.TrackPrimaryScenicSpot{TrackID: spot.TrackID, ID: spot.ID, Provider: source.Provider, ProviderID: source.ProviderID, Category: spot.Category}
			if source.Provider == "amap" {
				s.amapScenicDetails(ctx, candidate)
			} else if source.Provider == "google" {
				s.googleScenicDetails(ctx, candidate)
			}
			if spot.Address == "" {
				spot.Address = candidate.Address
			}
			if spot.Description == "" {
				spot.Description = candidate.Description
			}
			if spot.Category == "" {
				spot.Category = candidate.Category
			}
			for _, photo := range candidate.Photos {
				if len(spot.Photos) >= 3 {
					break
				}
				spot.Photos = append(spot.Photos, photo)
			}
		}
		return
	}
	if spot.ProviderID == "" || spot.Provider == "" {
		return
	}
	if spot.Provider == "amap" {
		s.amapScenicDetails(ctx, spot)
		return
	}
	if spot.Provider == "google" {
		s.googleScenicDetails(ctx, spot)
	}
}

func (s *TrackPlaceService) reserveScenicCall(ctx context.Context, provider string) bool {
	budget := s.cfg.AMapDailyBudget
	if provider == "google" {
		budget = s.cfg.GoogleDailyBudget
	}
	ok, err := s.store.ReservePlaceAPICall(ctx, provider, time.Now().In(placeQuotaZone).Format("2006-01-02"), budget)
	return err == nil && ok
}

func (s *TrackPlaceService) recordScenicProviderCall(ctx context.Context, spotID, provider, operation string, succeeded bool) {
	if s != nil && s.scenicCatalog != nil {
		s.scenicCatalog.RecordProviderUsage(ctx, spotID, provider, operation, succeeded)
	}
}

func (s *TrackPlaceService) recordScenicTrackProviderCall(ctx context.Context, trackID, provider, operation string, succeeded bool) {
	if s != nil && s.scenicCatalog != nil {
		s.scenicCatalog.RecordProviderUsageForTrack(ctx, trackID, provider, operation, succeeded)
	}
}

func (s *TrackPlaceService) recordScenicDetailsCall(ctx context.Context, spot *models.TrackPrimaryScenicSpot, provider, operation string, succeeded bool) {
	if spot == nil {
		return
	}
	if spot.TrackID != "" {
		s.recordScenicTrackProviderCall(ctx, spot.TrackID, provider, operation, succeeded)
		return
	}
	s.recordScenicProviderCall(ctx, spot.ID, provider, operation, succeeded)
}

// ScenicSpotProviderPreview fetches provider text and photo references for the
// scenic catalog import and admin display flows.
func (s *TrackPlaceService) ScenicSpotProviderPreview(ctx context.Context, spotID, provider, providerID string) (*models.TrackPrimaryScenicSpot, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || (provider != "amap" && provider != "google") {
		return nil, errors.New("provider details unavailable")
	}
	if !s.Enabled() {
		return nil, errors.New("scenic map service is not enabled")
	}
	if provider == "amap" && s.cfg.AMapKey == "" {
		return nil, errors.New("AMap key is not configured")
	}
	if provider == "google" && s.cfg.GoogleKey == "" {
		return nil, errors.New("Google Places key is not configured")
	}
	spot := &models.TrackPrimaryScenicSpot{ID: spotID, Provider: provider, ProviderID: providerID}
	switch provider {
	case "amap":
		s.amapScenicDetails(ctx, spot)
	case "google":
		s.googleScenicDetails(ctx, spot)
	}
	if spot.Name == "" && spot.Category == "" && spot.Address == "" && spot.Description == "" && len(spot.Photos) == 0 {
		if spot.Latitude != 0 && spot.Longitude != 0 {
			return spot, nil
		}
		return nil, errors.New("provider returned no details; check the place ID, API permission, and quota")
	}
	return spot, nil
}

type ScenicSpotGoogleDescription struct {
	Description string  `json:"description"`
	PlaceName   string  `json:"place_name,omitempty"`
	DistanceM   float64 `json:"distance_m,omitempty"`
}

// LookupScenicSpotGoogleDescription performs an explicit, uncached Google
// Places lookup for the admin's description helper. It uses a linked Google
// place ID when one exists; otherwise it searches by the catalog name and
// location and accepts only a nearby, name-matching result with a summary.
func (s *TrackPlaceService) LookupScenicSpotGoogleDescription(ctx context.Context, spot *models.ScenicSpot) (*ScenicSpotGoogleDescription, error) {
	if s == nil || !s.Enabled() || s.cfg.GoogleKey == "" {
		return nil, errors.New("Google Places is not configured")
	}
	if spot == nil || strings.TrimSpace(spot.Name) == "" {
		return nil, errors.New("scenic spot name is required")
	}
	if spot.Latitude == 0 || spot.Longitude == 0 {
		return nil, errors.New("scenic spot coordinates are required for Google matching")
	}
	var result *ScenicSpotGoogleDescription
	var err error
	googleID := ""
	for _, source := range spot.Sources {
		if source.Provider == "google" && source.Status == "active" && strings.TrimSpace(source.ProviderID) != "" {
			googleID = strings.TrimSpace(source.ProviderID)
			break
		}
	}
	if googleID != "" {
		result, err = s.googleScenicDescriptionByID(ctx, googleID)
	} else {
		result, err = s.searchGoogleScenicDescription(ctx, spot)
	}
	s.recordScenicProviderCall(ctx, spot.ID, "google", "description", err == nil)
	return result, err
}

func (s *TrackPlaceService) googleScenicDescriptionByID(ctx context.Context, placeID string) (*ScenicSpotGoogleDescription, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://places.googleapis.com/v1/places/"+url.PathEscape(placeID), nil)
	req.Header.Set("X-Goog-Api-Key", s.cfg.GoogleKey)
	req.Header.Set("X-Goog-FieldMask", "displayName,editorialSummary")
	var response struct {
		DisplayName struct {
			Text string `json:"text"`
		} `json:"displayName"`
		EditorialSummary struct {
			Text string `json:"text"`
		} `json:"editorialSummary"`
	}
	if err := s.doProviderRequest(ctx, req, &response); err != nil {
		return nil, err
	}
	return &ScenicSpotGoogleDescription{Description: strings.TrimSpace(response.EditorialSummary.Text), PlaceName: strings.TrimSpace(response.DisplayName.Text)}, nil
}

func (s *TrackPlaceService) searchGoogleScenicDescription(ctx context.Context, spot *models.ScenicSpot) (*ScenicSpotGoogleDescription, error) {
	point := models.TrackPoint{Latitude: spot.Latitude, Longitude: spot.Longitude}
	if strings.EqualFold(spot.CoordinateSystem, "GCJ02") || strings.EqualFold(spot.CoordinateSystem, "GCJ-02") {
		point = mapGCJ02ToWGS84(point)
	}
	textQuery := strings.TrimSpace(spot.Name)
	if city := strings.TrimSpace(spot.CityName); city != "" {
		textQuery += " " + city
	}
	body, _ := json.Marshal(map[string]interface{}{
		"textQuery":      textQuery,
		"languageCode":   "zh-CN",
		"maxResultCount": 5,
		"rankPreference": "DISTANCE",
		"locationBias": map[string]interface{}{"circle": map[string]interface{}{
			"center": map[string]float64{"latitude": point.Latitude, "longitude": point.Longitude},
			"radius": 5000,
		}},
	})
	req, _ := http.NewRequest(http.MethodPost, "https://places.googleapis.com/v1/places:searchText", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", s.cfg.GoogleKey)
	req.Header.Set("X-Goog-FieldMask", "places.displayName,places.location,places.editorialSummary")
	var response struct {
		Places []struct {
			DisplayName struct {
				Text string `json:"text"`
			} `json:"displayName"`
			Location struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
			EditorialSummary struct {
				Text string `json:"text"`
			} `json:"editorialSummary"`
		} `json:"places"`
	}
	if err := s.doProviderRequest(ctx, req, &response); err != nil {
		return nil, err
	}
	spotName := normalizeScenicMatchName(spot.Name)
	var best *ScenicSpotGoogleDescription
	for _, place := range response.Places {
		name := strings.TrimSpace(place.DisplayName.Text)
		summary := strings.TrimSpace(place.EditorialSummary.Text)
		googleName := normalizeScenicMatchName(name)
		if summary == "" || name == "" || (googleName != spotName && !strings.Contains(googleName, spotName) && !strings.Contains(spotName, googleName)) {
			continue
		}
		distance := haversineMeters(point.Latitude, point.Longitude, place.Location.Latitude, place.Location.Longitude)
		if distance > 5000 || (best != nil && distance >= best.DistanceM) {
			continue
		}
		best = &ScenicSpotGoogleDescription{Description: summary, PlaceName: name, DistanceM: distance}
	}
	if best == nil {
		return nil, errors.New("Google Maps did not return a matching place description")
	}
	return best, nil
}

func normalizeScenicMatchName(value string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if !unicode.IsSpace(r) && !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

// ScenicSpotProviderCoordinates refreshes only the provider's point coordinate.
// It deliberately avoids photo media calls when syncing already-imported records.
func (s *TrackPlaceService) ScenicSpotProviderCoordinates(ctx context.Context, spotID, provider, providerID string) (float64, float64, string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || (provider != "amap" && provider != "google") || !s.Enabled() {
		return 0, 0, "", errors.New("provider coordinates unavailable")
	}
	if provider == "amap" {
		if s.cfg.AMapKey == "" || !s.reserveScenicCall(ctx, provider) {
			return 0, 0, "", errors.New("AMap coordinates unavailable")
		}
		q := url.Values{"key": {s.cfg.AMapKey}, "id": {providerID}, "extensions": {"base"}, "output": {"json"}}
		req, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/v3/place/detail?"+q.Encode(), nil)
		var result struct {
			Status string `json:"status"`
			Pois   []struct {
				Location amapPlaceText `json:"location"`
			} `json:"pois"`
		}
		err := s.doProviderRequest(ctx, req, &result)
		var lat, lng float64
		if err == nil && result.Status == "1" && len(result.Pois) > 0 {
			_, err = fmt.Sscanf(string(result.Pois[0].Location), "%f,%f", &lng, &lat)
		}
		ok := err == nil && validProviderCoordinate(lat, lng)
		s.recordScenicProviderCall(ctx, spotID, provider, "coordinates", ok)
		if !ok {
			return 0, 0, "", errors.New("AMap coordinates unavailable")
		}
		return lat, lng, "GCJ02", nil
	}
	if s.cfg.GoogleKey == "" || !s.reserveScenicCall(ctx, provider) {
		return 0, 0, "", errors.New("Google Places coordinates unavailable")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://places.googleapis.com/v1/places/"+url.PathEscape(providerID), nil)
	req.Header.Set("X-Goog-Api-Key", s.cfg.GoogleKey)
	req.Header.Set("X-Goog-FieldMask", "location")
	var result struct {
		Location struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"location"`
	}
	err := s.doProviderRequest(ctx, req, &result)
	ok := err == nil && validProviderCoordinate(result.Location.Latitude, result.Location.Longitude)
	s.recordScenicProviderCall(ctx, spotID, provider, "coordinates", ok)
	if !ok {
		return 0, 0, "", errors.New("Google Places coordinates unavailable")
	}
	return result.Location.Latitude, result.Location.Longitude, "WGS84", nil
}

func validProviderCoordinate(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 && lat != 0 && lng != 0
}

// ScenicSpotStaticMap returns a small authenticated proxy image so the admin
// page does not depend on third-party iframe availability or expose map keys.
func (s *TrackPlaceService) ScenicSpotStaticMap(ctx context.Context, spotID string, lat, lng float64, coordinateSystem string) ([]byte, string, error) {
	if s == nil || s.cfg.AMapKey == "" || s.client == nil || !validProviderCoordinate(lat, lng) {
		return nil, "", errors.New("scenic spot map unavailable")
	}
	point, err := mapPointToGCJ02(models.TrackPoint{Latitude: lat, Longitude: lng}, coordinateSystem)
	if err != nil {
		return nil, "", errors.New("scenic spot map unavailable")
	}
	if s.store != nil && !s.reserveScenicCall(ctx, "amap") {
		return nil, "", errors.New("scenic spot map unavailable")
	}
	location := fmt.Sprintf("%.6f,%.6f", point.Longitude, point.Latitude)
	q := url.Values{
		"key":      {s.cfg.AMapKey},
		"location": {location},
		"zoom":     {"15"},
		"size":     {"640*220"},
		"markers":  {"mid,0xE55353,A:" + location},
	}
	req, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/v3/staticmap?"+q.Encode(), nil)
	resp, err := s.client.Do(req.WithContext(ctx))
	if err != nil {
		s.recordScenicProviderCall(ctx, spotID, "amap", "static_map", false)
		return nil, "", errors.New("scenic spot map unavailable")
	}
	defer resp.Body.Close()
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || (contentType != "image/png" && contentType != "image/jpeg") {
		s.recordScenicProviderCall(ctx, spotID, "amap", "static_map", false)
		return nil, "", errors.New("scenic spot map unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || len(body) == 0 {
		s.recordScenicProviderCall(ctx, spotID, "amap", "static_map", false)
		return nil, "", errors.New("scenic spot map unavailable")
	}
	s.recordScenicProviderCall(ctx, spotID, "amap", "static_map", true)
	return body, contentType, nil
}

func (s *TrackPlaceService) ScenicSpotMapJSKey() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.cfg.AMapWebKey)
}

func (s *TrackPlaceService) ProxyAMapJSAPI(ctx context.Context, requestPath string, query url.Values) ([]byte, string, error) {
	if s == nil || s.cfg.AMapSecurityJSCode == "" || s.client == nil {
		return nil, "", errors.New("AMap JS API proxy unavailable")
	}
	requestPath = strings.TrimPrefix(requestPath, "/")
	cleanPath := path.Clean(requestPath)
	if requestPath == "" || cleanPath == "." || strings.HasPrefix(cleanPath, "../") || strings.Contains(cleanPath, "\\") {
		return nil, "", errors.New("AMap JS API proxy path invalid")
	}
	host := "https://restapi.amap.com/"
	if strings.HasPrefix(cleanPath, "v4/map/styles/") {
		host = "https://webapi.amap.com/"
	}
	values := url.Values{}
	for key, items := range query {
		for _, item := range items {
			values.Add(key, item)
		}
	}
	values.Set("jscode", s.cfg.AMapSecurityJSCode)
	req, _ := http.NewRequest(http.MethodGet, host+cleanPath+"?"+values.Encode(), nil)
	resp, err := s.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, "", errors.New("AMap JS API proxy request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", errors.New("AMap JS API proxy request failed")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil || len(body) == 0 {
		return nil, "", errors.New("AMap JS API proxy response invalid")
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return body, contentType, nil
}

func (s *TrackPlaceService) VerifyScenicSpotSource(ctx context.Context, spotID, provider, providerID string) error {
	if !s.Enabled() || strings.TrimSpace(providerID) == "" {
		return errors.New("place provider unavailable")
	}
	if provider == "google" {
		if s.cfg.GoogleKey == "" || !s.reserveScenicCall(ctx, "google") {
			return errors.New("google place verification unavailable")
		}
		req, _ := http.NewRequest(http.MethodGet, "https://places.googleapis.com/v1/places/"+url.PathEscape(providerID), nil)
		req.Header.Set("X-Goog-Api-Key", s.cfg.GoogleKey)
		req.Header.Set("X-Goog-FieldMask", "id")
		var result struct {
			ID string `json:"id"`
		}
		err := s.doProviderRequest(ctx, req, &result)
		s.recordScenicProviderCall(ctx, spotID, provider, "verify", err == nil && result.ID == providerID)
		if err != nil {
			return err
		}
		if result.ID == "" || result.ID != providerID {
			return errors.New("google place id did not match")
		}
		return nil
	}
	if provider == "amap" {
		if s.cfg.AMapKey == "" || !s.reserveScenicCall(ctx, "amap") {
			return errors.New("amap place verification unavailable")
		}
		q := url.Values{"key": {s.cfg.AMapKey}, "id": {providerID}, "extensions": {"base"}, "output": {"json"}}
		req, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/v3/place/detail?"+q.Encode(), nil)
		var result struct {
			Status string `json:"status"`
			Pois   []struct {
				ID amapPlaceText `json:"id"`
			} `json:"pois"`
		}
		err := s.doProviderRequest(ctx, req, &result)
		succeeded := err == nil && result.Status == "1" && len(result.Pois) > 0 && string(result.Pois[0].ID) == providerID
		s.recordScenicProviderCall(ctx, spotID, provider, "verify", succeeded)
		if err != nil {
			return err
		}
		if result.Status != "1" || len(result.Pois) == 0 || string(result.Pois[0].ID) != providerID {
			return errors.New("amap place id was not found")
		}
		return nil
	}
	return errors.New("provider must be amap or google")
}

func (s *TrackPlaceService) amapScenicDetails(ctx context.Context, spot *models.TrackPrimaryScenicSpot) {
	if !s.Enabled() || s.cfg.AMapKey == "" || !s.reserveScenicCall(ctx, "amap") {
		return
	}
	q := url.Values{"key": {s.cfg.AMapKey}, "id": {spot.ProviderID}, "extensions": {"all"}, "output": {"json"}}
	req, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/v3/place/detail?"+q.Encode(), nil)
	var result struct {
		Status string `json:"status"`
		Pois   []struct {
			Name     string        `json:"name"`
			Address  string        `json:"address"`
			Type     string        `json:"type"`
			Location amapPlaceText `json:"location"`
			Photos   []struct {
				Title amapPlaceText `json:"title"`
				URL   amapPlaceText `json:"url"`
			} `json:"photos"`
		} `json:"pois"`
	}
	err := s.doProviderRequest(ctx, req, &result)
	succeeded := err == nil && result.Status == "1" && len(result.Pois) > 0
	s.recordScenicDetailsCall(ctx, spot, "amap", "details", succeeded)
	if !succeeded {
		return
	}
	poi := result.Pois[0]
	if spot.Name == "" {
		spot.Name = strings.TrimSpace(poi.Name)
	}
	spot.Address = strings.TrimSpace(poi.Address)
	if spot.Category == "" {
		spot.Category = strings.TrimSpace(poi.Type)
	}
	var lat, lng float64
	if _, err := fmt.Sscanf(string(poi.Location), "%f,%f", &lng, &lat); err == nil && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 && lat != 0 && lng != 0 {
		spot.Latitude, spot.Longitude, spot.CoordinateSystem = lat, lng, "GCJ02"
	}
	for _, photo := range poi.Photos {
		if len(spot.Photos) == 3 {
			break
		}
		if photoURL := safeAMapPhotoURL(string(photo.URL)); photoURL != "" {
			spot.Photos = append(spot.Photos, models.TrackScenicSpotPhoto{URL: photoURL, Title: strings.TrimSpace(string(photo.Title)), SourceURL: "https://www.amap.com/"})
		}
	}
}

func safeAMapPhotoURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "amap.com" && !strings.HasSuffix(host, ".amap.com") && host != "autonavi.com" && !strings.HasSuffix(host, ".autonavi.com") {
		return ""
	}
	u.Scheme = "https"
	return u.String()
}

func (s *TrackPlaceService) googleScenicDetails(ctx context.Context, spot *models.TrackPrimaryScenicSpot) {
	if !s.Enabled() || s.cfg.GoogleKey == "" || !s.reserveScenicCall(ctx, "google") {
		return
	}
	u := "https://places.googleapis.com/v1/places/" + url.PathEscape(spot.ProviderID)
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("X-Goog-Api-Key", s.cfg.GoogleKey)
	req.Header.Set("X-Goog-FieldMask", "id,displayName,primaryType,formattedAddress,editorialSummary,googleMapsUri,location,photos.name,photos.authorAttributions")
	var result struct {
		DisplayName struct {
			Text string `json:"text"`
		} `json:"displayName"`
		PrimaryType      string `json:"primaryType"`
		FormattedAddress string `json:"formattedAddress"`
		GoogleMapsURI    string `json:"googleMapsUri"`
		EditorialSummary struct {
			Text string `json:"text"`
		} `json:"editorialSummary"`
		Location struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"location"`
		Photos []struct {
			Name         string `json:"name"`
			Attributions []struct {
				DisplayName string `json:"displayName"`
				URI         string `json:"uri"`
			} `json:"authorAttributions"`
		} `json:"photos"`
	}
	err := s.doProviderRequest(ctx, req, &result)
	s.recordScenicDetailsCall(ctx, spot, "google", "details", err == nil)
	if err != nil {
		return
	}
	if spot.Name == "" {
		spot.Name = strings.TrimSpace(result.DisplayName.Text)
	}
	spot.Address, spot.Description = result.FormattedAddress, result.EditorialSummary.Text
	if result.Location.Latitude >= -90 && result.Location.Latitude <= 90 && result.Location.Longitude >= -180 && result.Location.Longitude <= 180 && result.Location.Latitude != 0 && result.Location.Longitude != 0 {
		spot.Latitude, spot.Longitude, spot.CoordinateSystem = result.Location.Latitude, result.Location.Longitude, "WGS84"
	}
	if spot.Category == "" {
		spot.Category = result.PrimaryType
	}
	for _, photo := range result.Photos {
		if len(spot.Photos) == 3 {
			break
		}
		if photo.Name == "" || !s.reserveScenicCall(ctx, "google") {
			break
		}
		mediaURL := "https://places.googleapis.com/v1/" + photo.Name + "/media?maxWidthPx=1200&skipHttpRedirect=true&key=" + url.QueryEscape(s.cfg.GoogleKey)
		mediaReq, _ := http.NewRequest(http.MethodGet, mediaURL, nil)
		var media struct {
			PhotoURI string `json:"photoUri"`
		}
		mediaErr := s.doProviderRequest(ctx, mediaReq, &media)
		mediaOK := mediaErr == nil && strings.HasPrefix(media.PhotoURI, "https://")
		s.recordScenicDetailsCall(ctx, spot, "google", "photo", mediaOK)
		if !mediaOK {
			continue
		}
		item := models.TrackScenicSpotPhoto{URL: media.PhotoURI, SourceURL: result.GoogleMapsURI}
		if len(photo.Attributions) > 0 {
			authors := make([]string, 0, len(photo.Attributions))
			for _, attribution := range photo.Attributions {
				if name := strings.TrimSpace(attribution.DisplayName); name != "" {
					authors = append(authors, name)
				}
			}
			item.Author = strings.Join(authors, ", ")
			item.AuthorURL = photo.Attributions[0].URI
		}
		spot.Photos = append(spot.Photos, item)
	}
}
