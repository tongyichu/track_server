package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const trackPlaceRuleVersion = 3
const trackPlaceEvidenceVersion = 1

var placeQuotaZone = time.FixedZone("Asia/Shanghai", 8*60*60)

type TrackPlaceConfig struct {
	Enabled                        bool
	AMapKey, GoogleKey             string
	AMapWebKey, AMapSecurityJSCode string
	AMapDailyBudget                int
	CacheTTL, GoogleCacheTTL       time.Duration
}
type TrackPlaceService struct {
	cfg           TrackPlaceConfig
	store         repository.TrackPlaceRepository
	maps          repository.TrackMapRepository
	tracks        repository.TrackRepository
	reader        repository.TrackPlaceIndexReader
	indexer       *TrackMapIndexService
	client        *http.Client
	mu            sync.Mutex
	lastCall      time.Time
	runCalls      int
	scenicCatalog *ScenicSpotCatalogService
}
type TrackPlaceRunResult struct{ Scanned, Matched, Candidates, Unmatched, Reused, Skipped, Ineligible, Deferred, QuotaDeferred, BusyDeferred, Failed, IndexRefreshFailed, InvalidSamples, Unconfigured, ProviderQueries, MissingIndex int }

// AMap enrichment is a one-time business result. Keep successful positive,
// ambiguous and negative results independently of the short-lived request cache.
func trackPlaceRecordFresh(r *models.TrackPlaceRecord) bool {
	if r == nil || r.Data == "{}" {
		return false
	}
	if r.ExpiresAt.IsZero() || r.ExpiresAt.After(time.Now()) {
		return true
	}
	if strings.HasPrefix(r.Key, "place:place-amap-") {
		var place models.TrackPlace
		return json.Unmarshal([]byte(r.Data), &place) == nil && place.Provider == "amap"
	}
	if strings.HasPrefix(r.Key, "match:") {
		var match models.TrackPlaceMatch
		return json.Unmarshal([]byte(r.Data), &match) == nil && match.Provider == "amap" && (match.Status == "matched" || match.Status == "candidate" || match.Status == "unmatched")
	}
	return false
}

func NewTrackPlaceService(cfg TrackPlaceConfig, store repository.TrackPlaceRepository, maps repository.TrackMapRepository, tracks repository.TrackRepository, indexer *TrackMapIndexService) *TrackPlaceService {
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 7 * 24 * time.Hour
	}
	if cfg.CacheTTL > 30*24*time.Hour {
		cfg.CacheTTL = 30 * 24 * time.Hour
	}
	if cfg.GoogleCacheTTL > 30*24*time.Hour {
		cfg.GoogleCacheTTL = 30 * 24 * time.Hour
	}
	reader, _ := maps.(repository.TrackPlaceIndexReader)
	return &TrackPlaceService{cfg: cfg, store: store, maps: maps, tracks: tracks, reader: reader, indexer: indexer, client: &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (s *TrackPlaceService) Enabled() bool {
	return s != nil && s.cfg.Enabled && s.reader != nil && s.store != nil
}

func (s *TrackPlaceService) SetScenicSpotCatalogService(catalog *ScenicSpotCatalogService) {
	if s != nil {
		s.scenicCatalog = catalog
	}
}
func (s *TrackPlaceService) owner() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}
func (s *TrackPlaceService) providerTTL(provider string) time.Duration {
	if provider == "google" {
		return s.cfg.GoogleCacheTTL
	}
	return s.cfg.CacheTTL
}
func (s *TrackPlaceService) loadFresh(ctx context.Context, key string, value interface{}) (bool, error) {
	r, err := s.store.GetPlaceRecord(ctx, key)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !trackPlaceRecordFresh(r) {
		return false, nil
	}
	if err := json.Unmarshal([]byte(r.Data), value); err != nil {
		return false, nil
	}
	return true, nil
}
func (s *TrackPlaceService) save(ctx context.Context, key string, value interface{}, expires time.Time, owner string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ok, err := s.store.SavePlaceRecord(ctx, &models.TrackPlaceRecord{Key: key, Data: string(data), ExpiresAt: expires}, owner)
	if err == nil && !ok {
		return errPlaceBusy
	}
	return err
}

// RunOnce scans bounded pages; its persistent cursor automatically includes
// historical tracks. Provider errors never stop uploads or viewport requests.
func (s *TrackPlaceService) RunOnce(ctx context.Context) (*TrackPlaceRunResult, error) {
	out := &TrackPlaceRunResult{}
	if !s.Enabled() {
		return out, nil
	}
	if !s.mu.TryLock() {
		return out, nil
	}
	defer s.mu.Unlock()
	if err := s.store.DeleteExpiredPlaceContent(ctx, time.Now()); err != nil {
		return out, err
	}
	s.runCalls = 0
	owner := s.owner()
	now := time.Now()
	leased, err := s.store.LeasePlaceRecord(ctx, "scan_lock", owner, now, now.Add(2*time.Minute))
	if err != nil {
		return out, err
	}
	if !leased {
		return out, nil
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.save(releaseCtx, "scan_lock", true, time.Time{}, owner)
	}()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var cursor string
	scanCursorKey := fmt.Sprintf("scan_cursor:v%d", trackPlaceRuleVersion)
	record, err := s.store.GetPlaceRecord(ctx, scanCursorKey)
	if err == nil {
		_ = json.Unmarshal([]byte(record.Data), &cursor)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return out, err
	}
	indexes, err := s.reader.ListPlaceGeoIndexes(ctx, cursor, 50)
	if err != nil {
		return out, err
	}
	if len(indexes) == 0 {
		return out, s.save(ctx, scanCursorKey, "", time.Time{}, "")
	}
	for _, index := range indexes {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out.Scanned++
		if err := s.process(ctx, index, out); err != nil {
			return out, err
		}
		if err := s.save(ctx, scanCursorKey, index.TrackID, time.Time{}, ""); err != nil {
			return out, err
		}
	}
	return out, nil
}

// ResolveTracksForBackfill retries only the explicitly selected legacy tracks.
// It shares the scanner's lock and AMap limits so an admin action cannot race
// the scheduled worker. Google lookups rely on Google API quotas.
func (s *TrackPlaceService) ResolveTracksForBackfill(ctx context.Context, trackIDs []string) (*TrackPlaceRunResult, error) {
	out := &TrackPlaceRunResult{}
	if !s.Enabled() {
		out.Unconfigured = len(trackIDs)
		return out, nil
	}
	if !s.mu.TryLock() {
		out.Deferred = len(trackIDs)
		out.BusyDeferred = len(trackIDs)
		return out, nil
	}
	defer s.mu.Unlock()
	s.runCalls = 0
	defer func() { out.ProviderQueries = s.runCalls }()
	for _, trackID := range trackIDs {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		index, err := s.maps.FindTrackGeoIndex(ctx, trackID)
		if errors.Is(err, repository.ErrNotFound) && s.indexer != nil {
			if indexErr := s.indexer.processJob(ctx, trackID); indexErr != nil {
				out.IndexRefreshFailed++
				out.MissingIndex++
				continue
			}
			index, err = s.maps.FindTrackGeoIndex(ctx, trackID)
		}
		if errors.Is(err, repository.ErrNotFound) {
			out.MissingIndex++
			continue
		}
		if err != nil {
			return out, err
		}
		out.Scanned++
		if err := s.processWithOptions(ctx, index, out, true); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *TrackPlaceService) process(ctx context.Context, index *models.TrackGeoIndex, out *TrackPlaceRunResult) error {
	return s.processWithOptions(ctx, index, out, false)
}

func (s *TrackPlaceService) processWithOptions(ctx context.Context, index *models.TrackGeoIndex, out *TrackPlaceRunResult, refreshMissing bool) error {
	track, err := s.tracks.FindByID(ctx, index.TrackID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !isTrackMapIndexEligible(track) {
		out.Ineligible++
		return nil
	}
	key := "match:" + index.TrackID
	var previous models.TrackPlaceMatch
	previousRecord, err := s.store.GetPlaceRecord(ctx, key)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	if previousRecord != nil {
		_ = json.Unmarshal([]byte(previousRecord.Data), &previous)
	}
	// Repair old mixed-CRS indexes with the full raw GPS, not their bounding boxes.
	// During the v1 migration, also inspect native GCJ02 exports once: the old
	// Android exporter wrote WGS84 despite declaring the session's source CRS.
	// Include WGS84 so an index write that failed after a successful metadata
	// correction is retried before the v1 evidence can be reused.
	nativeCoordinate := track.CoordinateSystem == "GCJ02" || track.CoordinateSystem == "WGS84"
	inspectNativeExport := track.SourceTag == "" && nativeCoordinate && previous.RuleVersion == 1 && (previous.Status == "matched" || previous.Status == "unmatched")
	if s.indexer != nil && (index.CoordinateSystem != "GCJ02" || inspectNativeExport) {
		if err := s.indexer.processJob(ctx, index.TrackID); err != nil {
			if refreshMissing {
				// Backfill can still resolve against the already persisted index.
				// Keep the failed refresh visible because the cached geometry may
				// carry an older coordinate correction.
				out.IndexRefreshFailed++
			} else {
				out.Failed++
				return nil
			}
		} else {
			index, err = s.maps.FindTrackGeoIndex(ctx, index.TrackID)
			if err != nil {
				return err
			}
			track, err = s.tracks.FindByID(ctx, index.TrackID)
			if errors.Is(err, repository.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if !isTrackMapIndexEligible(track) {
				out.Ineligible++
				return nil
			}
		}
	}
	samples, err := trackPlaceSamples(index, 5)
	if err != nil {
		out.InvalidSamples++
		return nil
	}
	if len(samples) < 3 {
		out.Skipped++
		return nil
	}
	provider := "amap"
	domestic := 0
	for _, p := range samples {
		if maparea.IsMainland(p.Latitude, p.Longitude) {
			domestic++
		}
	}
	if domestic != len(samples) {
		provider = "google"
	}
	if domestic > 0 && domestic < len(samples) {
		out.Unconfigured++
		return nil
	} // cross-border routes keep fallback.
	// Google content is not persisted unless an allowed cache lifetime has been
	// explicitly configured; a standard key alone grants no permanent database.
	if provider == "amap" && s.cfg.AMapKey == "" || provider == "google" && s.cfg.GoogleKey == "" {
		out.Unconfigured++
		return nil
	}
	source := trackPlaceSourceVersion(index)
	if !refreshMissing && trackPlaceRecordFresh(previousRecord) && previous.SourceVersion == source && previous.RuleVersion == trackPlaceRuleVersion {
		if previous.Status == "matched" || previous.Status == "candidate" || previous.Status == "unmatched" {
			out.Reused++
		} else {
			out.Deferred++
		}
		return s.refreshGroup(ctx, index, &previous)
	}
	if previous.SourceVersion != source {
		previous = models.TrackPlaceMatch{}
	}
	owner := s.owner()
	now := time.Now()
	ok, err := s.store.LeasePlaceRecord(ctx, key, owner, now, now.Add(2*time.Minute))
	if err != nil {
		return err
	}
	if !ok {
		out.Deferred++
		out.BusyDeferred++
		return nil
	}
	match := models.TrackPlaceMatch{TrackID: index.TrackID, SourceVersion: source, RuleVersion: trackPlaceRuleVersion, Provider: provider, Status: "unmatched", Relations: []models.TrackPlaceRelation{}, Attempts: previous.Attempts + 1, UpdatedAt: now}
	reuseEvidence := !refreshMissing && previous.SourceVersion == source && previous.Provider == provider && (previous.EvidenceComplete || (previous.RuleVersion == 1 && (previous.Status == "matched" || previous.Status == "unmatched")))
	var lookupErr error
	if reuseEvidence {
		match.Relations = append(match.Relations, previous.Relations...)
	} else {
		type votes struct {
			place        models.TrackPlace
			hits, inside int
		}
		candidates := map[string]*votes{}
		for _, p := range samples {
			values, e := s.providerLookupWithOptions(ctx, provider, p, refreshMissing)
			if e != nil {
				lookupErr = e
				break
			}
			for _, v := range values {
				item := candidates[v.Place.ID]
				if item == nil {
					item = &votes{place: v.Place}
					candidates[v.Place.ID] = item
				}
				item.hits++
				if v.Inside {
					item.inside++
				}
			}
		}
		if lookupErr == nil {
			ids := make([]string, 0, len(candidates))
			for id := range candidates {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				v := candidates[id]
				kind, count := "nearby", v.hits
				if v.inside > 0 {
					kind, count = "sampled_inside", v.inside
				}
				match.Relations = append(match.Relations, models.TrackPlaceRelation{PlaceID: id, Kind: kind, MatchedSamples: count, SampleCount: len(samples), Confidence: float64(count) / float64(len(samples))})
				if provider == "amap" {
					v.place.ExpiresAt = time.Time{}
				}
				if err := s.save(ctx, "place:"+id, &v.place, v.place.ExpiresAt, ""); err != nil {
					return err
				}
			}
		}
	}
	if lookupErr == nil {
		match.EvidenceComplete = true
		var preserved *models.TrackPlaceMatch
		if reuseEvidence {
			preserved = &previous
		}
		lookupErr = s.decideTrackPlace(ctx, track, index, &match, preserved)
	}
	expires := now.Add(s.providerTTL(provider))
	if lookupErr != nil {
		match.Status = "error"
		match.ErrorCode = lookupErr.Error()
		out.Failed++
		if errors.Is(lookupErr, errPlaceBudget) {
			match.Status = "quota_deferred"
			out.Deferred++
			out.QuotaDeferred++
			out.Failed--
			expires = nextPlaceQuotaDay(now)
		} else if errors.Is(lookupErr, errPlaceBusy) {
			out.Deferred++
			out.BusyDeferred++
			out.Failed--
			expires = now.Add(2 * time.Minute)
		} else {
			delay := retryDelay(match.Attempts)
			if delay < time.Minute {
				delay = time.Minute
			}
			if delay > 6*time.Hour {
				delay = 6 * time.Hour
			}
			expires = now.Add(delay)
		}
	} else {
		if provider == "amap" {
			expires = time.Time{}
		}
		if match.PrimaryPlaceID != "" {
			match.Status = "matched"
			out.Matched++
		} else if provider == "amap" && hasEnclosedPlaceCandidate(match.Relations) {
			match.Status = "candidate"
			out.Candidates++
		} else {
			out.Unmatched++
			if len(match.Relations) == 0 && expires.Sub(now) > 24*time.Hour {
				expires = now.Add(24 * time.Hour)
			}
		}
	}
	// A changed/deleted track must never receive the result of an older task.
	latest, err := s.maps.FindTrackGeoIndex(ctx, index.TrackID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	current, err := s.tracks.FindByID(ctx, index.TrackID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !isTrackMapIndexEligible(current) || current.RawTrackURL != track.RawTrackURL || current.CoordinateSystem != track.CoordinateSystem || current.TrackType != track.TrackType || trackPlaceSourceVersion(latest) != source {
		return nil
	}
	if err := s.save(ctx, key, &match, expires, owner); err != nil {
		return err
	}
	if match.Status == "matched" && s.scenicCatalog != nil {
		if err := s.scenicCatalog.SyncLegacyTrack(ctx, index.TrackID); err != nil {
			log.Printf("scenic spot catalog link failed track_id=%s", index.TrackID)
		}
	}
	return s.refreshGroup(ctx, index, &match)
}
func nextPlaceQuotaDay(now time.Time) time.Time {
	local := now.In(placeQuotaZone)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, placeQuotaZone)
}

func (s *TrackPlaceService) GetPlaces(ctx context.Context, ids []string) (map[string]*models.TrackPlace, error) {
	out := map[string]*models.TrackPlace{}
	for start := 0; start < len(ids); start += 200 {
		end := start + 200
		if end > len(ids) {
			end = len(ids)
		}
		keys := []string{}
		for _, id := range ids[start:end] {
			keys = append(keys, "place:"+id)
		}
		records, err := s.store.GetPlaceRecords(ctx, keys)
		if err != nil {
			return nil, err
		}
		for _, r := range records {
			if !trackPlaceRecordFresh(r) {
				continue
			}
			var place models.TrackPlace
			if json.Unmarshal([]byte(r.Data), &place) == nil && place.ID != "" {
				if place.Provider == "amap" {
					place.ExpiresAt = time.Time{}
				}
				out[place.ID] = &place
			}
		}
	}
	return out, nil
}

// Only DB reads here. Route-group rebuilds never call external providers.
func (s *TrackPlaceService) ApplyGroupAreas(ctx context.Context, groups []*models.TrackRouteGroup, indexes []*models.TrackGeoIndex) error {
	if !s.Enabled() {
		return nil
	}
	byID := map[string]*models.TrackGeoIndex{}
	for _, i := range indexes {
		byID[i.TrackID] = i
	}
	for start := 0; start < len(groups); start += 200 {
		end := start + 200
		if end > len(groups) {
			end = len(groups)
		}
		keys := []string{}
		matches := map[string]*models.TrackPlaceMatch{}
		placeIDs := []string{}
		for _, g := range groups[start:end] {
			keys = append(keys, "match:"+g.RepresentativeTrackID)
		}
		records, err := s.store.GetPlaceRecords(ctx, keys)
		if err != nil {
			return err
		}
		for _, g := range groups[start:end] {
			r := records["match:"+g.RepresentativeTrackID]
			index := byID[g.RepresentativeTrackID]
			if !trackPlaceRecordFresh(r) || index == nil {
				continue
			}
			var match models.TrackPlaceMatch
			if json.Unmarshal([]byte(r.Data), &match) != nil || match.SourceVersion != trackPlaceSourceVersion(index) || match.Status != "matched" {
				continue
			}
			matches[g.GroupID] = &match
			placeIDs = append(placeIDs, match.PrimaryPlaceID)
		}
		places, err := s.GetPlaces(ctx, placeIDs)
		if err != nil {
			return err
		}
		for _, g := range groups[start:end] {
			if m := matches[g.GroupID]; m != nil && places[m.PrimaryPlaceID] != nil {
				g.AreaID = m.PrimaryPlaceID
			}
		}
	}
	return nil
}
func (s *TrackPlaceService) refreshGroup(ctx context.Context, index *models.TrackGeoIndex, match *models.TrackPlaceMatch) error {
	writer, ok := s.maps.(repository.TrackPlaceGroupWriter)
	if !ok {
		return nil
	}
	g, err := s.maps.FindRouteGroupByTrackID(ctx, index.TrackID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if g.RepresentativeTrackID != index.TrackID {
		return nil
	}
	areaID := ""
	if district := maparea.DefaultCatalog().ResolveDistrict(g.CenterLat, g.CenterLng); district != nil {
		areaID = district.ID
	}
	if match.Status == "matched" && match.PrimaryPlaceID != "" {
		places, err := s.GetPlaces(ctx, []string{match.PrimaryPlaceID})
		if err != nil {
			return err
		}
		if places[match.PrimaryPlaceID] != nil {
			areaID = match.PrimaryPlaceID
		}
	}
	if g.AreaID == areaID {
		return nil
	}
	return writer.UpdateRouteGroupPlaceArea(ctx, g.GroupID, index.TrackID, areaID)
}

func (s *TrackPlaceService) NearbyPlaces(ctx context.Context, trackID string) ([]*models.TrackPlace, error) {
	var match models.TrackPlaceMatch
	fresh, err := s.loadFresh(ctx, "match:"+trackID, &match)
	if err != nil || !fresh {
		return nil, err
	}
	index, err := s.maps.FindTrackGeoIndex(ctx, trackID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if match.SourceVersion != trackPlaceSourceVersion(index) {
		return nil, nil
	}
	ids := []string{}
	for _, r := range match.Relations {
		ids = append(ids, r.PlaceID)
	}
	places, err := s.GetPlaces(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := []*models.TrackPlace{}
	for _, r := range match.Relations {
		if p := places[r.PlaceID]; p != nil {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.Compare(out[i].Name, out[j].Name) < 0 })
	if len(out) > 3 {
		out = out[:3]
	}
	return out, nil
}
