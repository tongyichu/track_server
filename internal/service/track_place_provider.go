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
	"strings"
	"time"

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
	if bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte("[]")) {
		*t = ""
		return nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	*t = amapPlaceText(v)
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
	if provider == "google" {
		p = mapGCJ02ToWGS84(p)
	}
	key := fmt.Sprintf("request:%d:%s:%.6f:%.6f", trackPlaceEvidenceVersion, provider, p.Latitude, p.Longitude)
	return s.cachedPlaceLookup(ctx, provider, key, func() ([]placeEvidence, error) {
		if provider == "amap" {
			return s.amapLookup(ctx, p)
		}
		return s.googleLookup(ctx, p)
	})
}

func (s *TrackPlaceService) cachedPlaceLookup(ctx context.Context, provider, key string, lookup func() ([]placeEvidence, error)) ([]placeEvidence, error) {
	var cached []placeEvidence
	if ok, err := s.loadFresh(ctx, key, &cached); err != nil {
		return nil, err
	} else if ok {
		return cached, nil
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
	if ok, err := s.loadFresh(ctx, key, &cached); err != nil {
		return nil, err
	} else if ok {
		if record, e := s.store.GetPlaceRecord(ctx, key); e == nil {
			_, _ = s.store.SavePlaceRecord(ctx, record, owner)
		}
		return cached, nil
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
