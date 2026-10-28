package repository

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/tongyichu/track_server/internal/models"
)

type MemoryScenicSpotRepository struct {
	mu     sync.RWMutex
	spots  map[string]*models.ScenicSpot
	links  map[string]*models.TrackScenicSpotLink
	audits map[string][]*models.ScenicSpotAudit
	usage  map[string]*models.ScenicSpotProviderUsage
}

func NewMemoryScenicSpotRepository() *MemoryScenicSpotRepository {
	return &MemoryScenicSpotRepository{spots: map[string]*models.ScenicSpot{}, links: map[string]*models.TrackScenicSpotLink{}, audits: map[string][]*models.ScenicSpotAudit{}, usage: map[string]*models.ScenicSpotProviderUsage{}}
}

func cloneScenic[T any](value *T) *T {
	if value == nil {
		return nil
	}
	b, _ := json.Marshal(value)
	var out T
	_ = json.Unmarshal(b, &out)
	return &out
}

func (r *MemoryScenicSpotRepository) CreateScenicSpot(_ context.Context, spot *models.ScenicSpot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spots[spot.ID] != nil {
		return ErrAlreadyExists
	}
	for _, existing := range r.spots {
		if existing.MergedInto == "" && spotHasSource(existing, spot.Sources) {
			return ErrAlreadyExists
		}
	}
	r.spots[spot.ID] = cloneScenic(spot)
	return nil
}
func (r *MemoryScenicSpotRepository) GetScenicSpot(_ context.Context, id string) (*models.ScenicSpot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if v := r.spots[id]; v != nil {
		return cloneScenic(v), nil
	}
	return nil, ErrNotFound
}
func (r *MemoryScenicSpotRepository) GetScenicSpots(_ context.Context, ids []string) (map[string]*models.ScenicSpot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]*models.ScenicSpot{}
	for _, id := range ids {
		if v := r.spots[id]; v != nil {
			out[id] = cloneScenic(v)
		}
	}
	return out, nil
}
func (r *MemoryScenicSpotRepository) FindScenicSpotBySource(_ context.Context, provider, providerID string) (*models.ScenicSpot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, spot := range r.spots {
		if spot.Status == models.ScenicSpotMerged {
			continue
		}
		for _, source := range spot.Sources {
			if source.Provider == provider && source.ProviderID == providerID {
				return cloneScenic(spot), nil
			}
		}
	}
	return nil, ErrNotFound
}
func (r *MemoryScenicSpotRepository) ListScenicSpots(_ context.Context, f ScenicSpotListFilter) ([]*models.ScenicSpot, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]*models.ScenicSpot, 0)
	for _, v := range r.spots {
		if f.Status != "" && v.Status != f.Status {
			continue
		}
		if f.Query != "" && !strings.Contains(strings.ToLower(v.Name), strings.ToLower(f.Query)) {
			continue
		}
		if f.Cursor != "" && v.ID <= f.Cursor {
			continue
		}
		all = append(all, cloneScenic(v))
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	total := int64(len(all))
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if len(all) > f.Limit {
		all = all[:f.Limit]
	}
	return all, total, nil
}
func (r *MemoryScenicSpotRepository) UpdateScenicSpot(_ context.Context, spot *models.ScenicSpot, expected int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.spots[spot.ID]
	if current == nil {
		return ErrNotFound
	}
	if current.Version != expected {
		return ErrConflict
	}
	for id, other := range r.spots {
		if id != spot.ID && other.MergedInto == "" && spotHasSource(other, spot.Sources) {
			return ErrAlreadyExists
		}
	}
	r.spots[spot.ID] = cloneScenic(spot)
	return nil
}
func (r *MemoryScenicSpotRepository) GetTrackScenicSpotLink(_ context.Context, trackID string) (*models.TrackScenicSpotLink, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if v := r.links[trackID]; v != nil {
		return cloneScenic(v), nil
	}
	return nil, ErrNotFound
}
func (r *MemoryScenicSpotRepository) GetTrackScenicSpotLinks(_ context.Context, trackIDs []string) (map[string]*models.TrackScenicSpotLink, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]*models.TrackScenicSpotLink{}
	for _, id := range trackIDs {
		if v := r.links[id]; v != nil {
			out[id] = cloneScenic(v)
		}
	}
	return out, nil
}
func (r *MemoryScenicSpotRepository) SetTrackScenicSpotLink(_ context.Context, link *models.TrackScenicSpotLink, expected string, absent bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.links[link.TrackID]
	if absent && current != nil {
		return ErrConflict
	}
	if !absent && (current == nil || current.SpotID != expected) {
		return ErrConflict
	}
	r.links[link.TrackID] = cloneScenic(link)
	return nil
}
func (r *MemoryScenicSpotRepository) DeleteTrackScenicSpotLink(_ context.Context, trackID, expected string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.links[trackID]
	if current == nil {
		return ErrNotFound
	}
	if current.SpotID != expected {
		return ErrConflict
	}
	delete(r.links, trackID)
	return nil
}
func (r *MemoryScenicSpotRepository) ListTrackScenicSpotLinks(_ context.Context, spotID, cursor string, limit int) ([]*models.TrackScenicSpotLink, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	out := []*models.TrackScenicSpotLink{}
	for _, v := range r.links {
		if v.SpotID == spotID && (cursor == "" || v.TrackID > cursor) {
			out = append(out, cloneScenic(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TrackID < out[j].TrackID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (r *MemoryScenicSpotRepository) AppendScenicSpotAudit(_ context.Context, audit *models.ScenicSpotAudit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audits[audit.SpotID] = append(r.audits[audit.SpotID], cloneScenic(audit))
	return nil
}
func (r *MemoryScenicSpotRepository) ListScenicSpotAudit(_ context.Context, spotID, cursor string, limit int) ([]*models.ScenicSpotAudit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	out := []*models.ScenicSpotAudit{}
	for _, v := range r.audits[spotID] {
		if cursor == "" || v.ID < cursor {
			out = append(out, cloneScenic(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func scenicUsageKey(spotID, provider, day, operation string) string {
	return spotID + "\x00" + provider + "\x00" + day + "\x00" + operation
}
func (r *MemoryScenicSpotRepository) IncrementScenicSpotProviderUsage(_ context.Context, spotID, provider, day, operation string, succeeded bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := scenicUsageKey(spotID, provider, day, operation)
	v := r.usage[key]
	if v == nil {
		v = &models.ScenicSpotProviderUsage{SpotID: spotID, Provider: provider, UsageDay: day, Operation: operation}
		r.usage[key] = v
	}
	v.Requests++
	if succeeded {
		v.Successes++
	} else {
		v.Failures++
	}
	return nil
}
func (r *MemoryScenicSpotRepository) ListScenicSpotProviderUsage(_ context.Context, fromDay, toDay string) ([]*models.ScenicSpotProviderUsage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []*models.ScenicSpotProviderUsage{}
	for _, v := range r.usage {
		if v.UsageDay >= fromDay && v.UsageDay <= toDay {
			out = append(out, cloneScenic(v))
		}
	}
	return out, nil
}
func (r *MemoryScenicSpotRepository) CountScenicSpotLinksBySpotIDs(_ context.Context, spotIDs []string) (map[string]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	wanted := make(map[string]bool, len(spotIDs))
	for _, id := range spotIDs {
		wanted[id] = true
	}
	out := make(map[string]int64, len(spotIDs))
	for _, link := range r.links {
		if wanted[link.SpotID] {
			out[link.SpotID]++
		}
	}
	return out, nil
}
func spotHasSource(existing *models.ScenicSpot, sources []models.ScenicSpotSource) bool {
	for _, a := range existing.Sources {
		for _, b := range sources {
			if a.Provider == b.Provider && a.ProviderID == b.ProviderID {
				return true
			}
		}
	}
	return false
}

var _ ScenicSpotRepository = (*MemoryScenicSpotRepository)(nil)
