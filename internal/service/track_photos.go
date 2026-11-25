package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/jpeg"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const PhotoBytesLimit = 200 * 1024
const PhotoSlots = 20

var photoIDPattern = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")

type TrackPhotoService struct {
	tracks repository.TrackRepository
	store  repository.TrackPhotoRepository
	spots  repository.ScenicSpotRepository
}

func NewTrackPhotoService(tracks repository.TrackRepository, spots repository.ScenicSpotRepository) *TrackPhotoService {
	return &TrackPhotoService{tracks, tracks.(repository.TrackPhotoRepository), spots}
}

type PhotoUpload struct {
	ID        string   `json:"id"`
	Slot      int      `json:"slot"`
	TakenAt   int64    `json:"taken_at_ms"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	JPEG      []byte   `json:"-"`
}

func (s *TrackPhotoService) track(ctx context.Context, track string, viewer int64) (*models.Track, error) {
	t, err := s.tracks.FindByID(ctx, track)
	if err != nil {
		return nil, err
	}
	if t.IsRunning || t.Status == models.TrackStatusDeleted || t.UserID != viewer && (t.Status != models.TrackStatusNormal || t.RawTrackURL == "") {
		return nil, repository.ErrNotFound
	}
	return t, nil
}
func (s *TrackPhotoService) Upload(ctx context.Context, track string, user int64, in PhotoUpload) (models.TrackPhoto, error) {
	var empty models.TrackPhoto
	t, err := s.track(ctx, track, user)
	if err != nil {
		return empty, err
	}
	if t.UserID != user {
		return empty, repository.ErrForbidden
	}
	if !photoIDPattern.MatchString(in.ID) || in.Slot < 1 || in.Slot > PhotoSlots || in.TakenAt <= 0 || len(in.JPEG) > PhotoBytesLimit || (in.Latitude == nil) != (in.Longitude == nil) || in.Latitude != nil && !validLatLng(*in.Latitude, *in.Longitude) {
		return empty, invalidArg("invalid photo metadata")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(in.JPEG))
	if err != nil || cfg.Width > 800 || cfg.Height > 800 {
		return empty, invalidArg("invalid JPEG size")
	}
	sum := sha256.Sum256(in.JPEG)
	p := models.TrackPhoto{ID: in.ID, TrackID: track, Slot: in.Slot, TakenAt: in.TakenAt, Latitude: in.Latitude, Longitude: in.Longitude, Status: "pending", Revision: 1, Digest: hex.EncodeToString(sum[:]), LocationChecked: in.Latitude == nil}
	img, err := jpeg.Decode(bytes.NewReader(in.JPEG))
	if err != nil {
		return empty, invalidArg("invalid JPEG")
	}
	var clean bytes.Buffer
	for _, q := range []int{88, 78, 65, 50, 40} {
		clean.Reset()
		if err = jpeg.Encode(&clean, img, &jpeg.Options{Quality: q}); err != nil {
			return empty, err
		}
		if clean.Len() <= 100*1024 {
			break
		}
	}
	if clean.Len() > PhotoBytesLimit {
		return empty, invalidArg("image too large")
	}
	if err = s.store.InsertPhoto(ctx, p, clean.Bytes()); errors.Is(err, repository.ErrAlreadyExists) {
		existing, _, readErr := s.store.ReadPhoto(ctx, in.ID, false)
		if readErr == nil && existing.TrackID == track && existing.Status != "removed" && existing.Slot == p.Slot && existing.Digest == p.Digest && existing.TakenAt == p.TakenAt && samePhotoCoord(existing.Latitude, p.Latitude) && samePhotoCoord(existing.Longitude, p.Longitude) {
			return photoView(existing, true), nil
		}
		return empty, repository.ErrConflict
	}
	return photoView(p, true), err
}
func samePhotoCoord(a, b *float64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
func photoView(p models.TrackPhoto, owner bool) models.TrackPhoto {
	p.Digest = ""
	p.InsideIDs = nil
	p.ScenicChoice = ""
	p.LocationChecked = false
	if p.Status != "rejected" || !owner {
		p.Reason = ""
	}
	if !owner {
		p.Slot = 0
	}
	return p
}
func (s *TrackPhotoService) List(ctx context.Context, track string, user int64) ([]models.TrackPhoto, error) {
	t, err := s.track(ctx, track, user)
	if err != nil {
		return nil, err
	}
	owner := t.UserID == user
	f := repository.PhotoFilter{TrackIDs: []string{track}, Limit: PhotoSlots}
	if !owner {
		f.Status = "approved"
	}
	items, err := s.store.PhotoPage(ctx, f)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].TakenAt < items[j].TakenAt })
	for i, p := range items {
		items[i] = photoView(p, owner)
	}
	return items, nil
}
func (s *TrackPhotoService) Media(ctx context.Context, track, id string, user int64, admin bool) ([]byte, error) {
	p, data, err := s.store.ReadPhoto(ctx, id, true)
	if err != nil {
		return nil, err
	}
	if p.Status == "removed" || track != "" && track != p.TrackID {
		return nil, repository.ErrNotFound
	}
	t, err := s.tracks.FindByID(ctx, p.TrackID)
	if err != nil {
		return nil, err
	}
	if t.IsRunning || t.Status == models.TrackStatusDeleted {
		return nil, repository.ErrNotFound
	}
	if !admin && t.UserID != user && (t.Status != models.TrackStatusNormal || t.RawTrackURL == "" || p.Status != "approved") {
		return nil, repository.ErrNotFound
	}
	return data, nil
}
func (s *TrackPhotoService) Remove(ctx context.Context, track, id string, user int64) error {
	t, err := s.track(ctx, track, user)
	if err != nil {
		return err
	}
	if t.UserID != user {
		return repository.ErrForbidden
	}
	for {
		p, _, err := s.store.ReadPhoto(ctx, id, false)
		if err != nil {
			return err
		}
		if p.TrackID != track {
			return repository.ErrNotFound
		}
		if p.Status == "removed" {
			return nil
		}
		p = models.TrackPhoto{ID: p.ID, TrackID: p.TrackID, Status: "removed", Revision: p.Revision}
		p.Revision++
		err = s.store.UpdatePhoto(ctx, p, p.Revision-1)
		if !errors.Is(err, repository.ErrConflict) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}
func (s *TrackPhotoService) Queue(ctx context.Context, status, after string) ([]models.TrackPhoto, string, error) {
	if status != "pending" && status != "approved" && status != "rejected" {
		return nil, "", invalidArg("invalid status")
	}
	const pageSize = 5
	ids, err := s.store.ReviewTrackIDs(ctx, status, after, pageSize+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(ids) > pageSize {
		ids = ids[:pageSize]
		next = ids[len(ids)-1]
	}
	items, err := s.store.PhotoPage(ctx, repository.PhotoFilter{TrackIDs: ids, Status: status, Limit: pageSize * PhotoSlots})
	sort.Slice(items, func(i, j int) bool {
		if items[i].TrackID == items[j].TrackID {
			return items[i].TakenAt < items[j].TakenAt
		}
		return items[i].TrackID < items[j].TrackID
	})
	for i := range items {
		items[i].Digest = ""
		if items[i].Status != "rejected" {
			items[i].Reason = ""
		}
	}
	return items, next, err
}
func (s *TrackPhotoService) PendingCount(ctx context.Context) (int64, error) {
	return s.store.PendingPhotos(ctx)
}

type PhotoDecision struct {
	Revision int64   `json:"revision"`
	Status   string  `json:"status"`
	Reason   string  `json:"reason"`
	Scenic   *string `json:"scenic_choice"`
}

func (s *TrackPhotoService) Review(ctx context.Context, id string, in PhotoDecision) error {
	p, _, err := s.store.ReadPhoto(ctx, id, false)
	if err != nil {
		return err
	}
	if p.Revision != in.Revision || p.Status == "removed" {
		return repository.ErrConflict
	}
	if in.Scenic != nil {
		if in.Status != "" || p.Status != "approved" {
			return invalidArg("photo must be approved")
		}
		choice := *in.Scenic
		if choice != "" {
			link, err := s.spots.GetTrackScenicSpotLink(ctx, p.TrackID)
			if err != nil {
				return err
			}
			if link.SpotID != strings.TrimPrefix(choice, "-") {
				return repository.ErrConflict
			}
		}
		p.ScenicChoice = choice
	} else {
		var reason string
		if in.Status == "rejected" {
			reason = strings.TrimSpace(in.Reason)
		}
		if in.Status != "approved" && in.Status != "rejected" || in.Status == "rejected" && reason == "" || utf8.RuneCountInString(reason) > 250 {
			return invalidArg("invalid review decision or reason")
		}
		p.Status = in.Status
		p.Reason = reason
	}
	p.Revision++
	return s.store.UpdatePhoto(ctx, p, in.Revision)
}
func photoInSpot(p models.TrackPhoto, spot *models.ScenicSpot) bool {
	if p.ScenicChoice == spot.ID {
		return true
	}
	if p.ScenicChoice == "-"+spot.ID {
		return false
	}
	for _, source := range spot.Sources {
		if source.Provider != "amap" || source.Status == "disabled" {
			continue
		}
		for _, id := range p.InsideIDs {
			if source.ProviderID == id {
				return true
			}
		}
	}
	return false
}
func (s *TrackPhotoService) CheckScenicPhoto(ctx context.Context, spot, id string) error {
	p, _, err := s.store.ReadPhoto(ctx, id, false)
	if err != nil {
		return err
	}
	if p.Status != "approved" {
		return repository.ErrNotFound
	}
	if _, err = s.track(ctx, p.TrackID, 0); err != nil {
		return err
	}
	link, err := s.spots.GetTrackScenicSpotLink(ctx, p.TrackID)
	if err != nil {
		return err
	}
	if link.SpotID != spot {
		return repository.ErrNotFound
	}
	return nil
}

func (s *TrackPhotoService) ScenicMedia(ctx context.Context, spotID, id string, includeCandidates bool) ([]byte, error) {
	p, data, err := s.store.ReadPhoto(ctx, id, true)
	if err != nil {
		return nil, err
	}
	if p.Status != "approved" {
		return nil, repository.ErrNotFound
	}
	if _, err = s.track(ctx, p.TrackID, 0); err != nil {
		return nil, err
	}
	link, err := s.spots.GetTrackScenicSpotLink(ctx, p.TrackID)
	if err != nil {
		return nil, err
	}
	if link.SpotID != spotID {
		return nil, repository.ErrNotFound
	}
	spot, err := s.spots.GetScenicSpot(ctx, spotID)
	if err != nil {
		return nil, err
	}
	if !includeCandidates && (!scenicSpotClientVisible(spot) || !photoInSpot(p, spot)) {
		return nil, repository.ErrNotFound
	}
	return data, nil
}

type ScenicPhotoItem struct {
	models.TrackPhoto
	Included bool  `json:"included"`
	IsMine   *bool `json:"is_mine,omitempty"`
}
type ScenicPhotoPage struct {
	Items []ScenicPhotoItem `json:"items"`
	Next  string            `json:"next_cursor,omitempty"`
}

func (s *TrackPhotoService) Scenic(ctx context.Context, id, after string, admin bool, viewer int64) (ScenicPhotoPage, error) {
	out := ScenicPhotoPage{Items: []ScenicPhotoItem{}}
	spot, err := s.spots.GetScenicSpot(ctx, id)
	if err != nil {
		return out, err
	}
	if !admin && !scenicSpotClientVisible(spot) {
		return out, repository.ErrNotFound
	}
	f := repository.PhotoFilter{Status: "approved", After: after, Limit: 25, Spot: id}
	if !admin {
		f.Sources = []string{}
		for _, source := range spot.Sources {
			if source.Provider == "amap" && source.Status != "disabled" {
				f.Sources = append(f.Sources, source.ProviderID)
			}
		}
	}
	photos, err := s.store.PhotoPage(ctx, f)
	if err != nil {
		return out, err
	}
	if len(photos) > 24 {
		photos = photos[:24]
		out.Next = photos[23].ID
	}
	owners := make(map[string]int64)
	if viewer > 0 && len(photos) > 0 {
		ids := make([]string, 0, len(photos))
		for _, photo := range photos {
			ids = append(ids, photo.TrackID)
		}
		tracks, err := s.tracks.FindByIDs(ctx, ids)
		if err != nil {
			return out, err
		}
		for id, track := range tracks {
			if track != nil {
				owners[id] = track.UserID
			}
		}
	}
	for _, p := range photos {
		item := ScenicPhotoItem{TrackPhoto: photoView(p, false), Included: photoInSpot(p, spot)}
		if owner, found := owners[p.TrackID]; found {
			mine := owner == viewer
			item.IsMine = &mine
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// Called with the existing place worker's quota lease and mutex.
func (s *TrackPhotoService) checkLocations(ctx context.Context, places *TrackPlaceService) error {
	if places.cfg.AMapKey == "" {
		return nil
	}
	var cursor string
	if _, err := places.loadFresh(ctx, "photo_location_cursor", &cursor); err != nil {
		return err
	}
	batch, err := s.store.PhotoPage(ctx, repository.PhotoFilter{Status: "approved", Unchecked: true, After: cursor, Limit: 2})
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(batch))
	for _, p := range batch {
		ids = append(ids, p.TrackID)
	}
	tracks, err := s.tracks.FindByIDs(ctx, ids)
	if err != nil {
		return err
	}
	links, err := s.spots.GetTrackScenicSpotLinks(ctx, ids)
	if err != nil {
		return err
	}
	spots := []string{}
	for _, link := range links {
		if link.SpotID != "" {
			spots = append(spots, link.SpotID)
		}
	}
	catalog, err := s.spots.GetScenicSpots(ctx, spots)
	if err != nil {
		return err
	}
	for _, p := range batch {
		t, link := tracks[p.TrackID], links[p.TrackID]
		cursor = p.ID
		if t == nil || link == nil || !scenicSpotClientVisible(catalog[link.SpotID]) {
			continue
		}
		if t.Status == models.TrackStatusNormal && !t.IsRunning && t.RawTrackURL != "" {
			if p.Latitude != nil && maparea.IsMainland(*p.Latitude, *p.Longitude) {
				pt, err := mapPointToGCJ02(models.TrackPoint{Latitude: *p.Latitude, Longitude: *p.Longitude}, "WGS84")
				if err != nil {
					return err
				}
				evidence, err := places.providerLookup(ctx, "amap", pt)
				if err != nil {
					if saveErr := places.save(ctx, "photo_location_cursor", p.ID, time.Time{}, ""); saveErr != nil {
						return saveErr
					}
					return err
				}
				for _, e := range evidence {
					if e.Inside {
						p.InsideIDs = append(p.InsideIDs, e.Place.ProviderID)
					}
				}
			}
			p.LocationChecked = true
			p.Revision++
			err = s.store.UpdatePhoto(ctx, p, p.Revision-1)
			if err != nil && !errors.Is(err, repository.ErrConflict) {
				return err
			}
		}
		cursor = p.ID
	}
	if len(batch) < 2 {
		cursor = ""
	}
	return places.save(ctx, "photo_location_cursor", cursor, time.Time{}, "")
}
