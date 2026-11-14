package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type CompanionRecordedPoint struct {
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	AltitudeMeters  float64 `json:"altitude_meters"`
	TimestampMillis int64   `json:"timestamp_millis"`
	SegmentID       int     `json:"segment_id"`
	AccuracyMeters  float64 `json:"accuracy_meters"`
	SpeedMps        float64 `json:"speed_mps"`
	Bearing         float64 `json:"bearing"`
}
type CompanionPointBatch struct {
	Index  int                      `json:"index"`
	Points []CompanionRecordedPoint `json:"points"`
}
type CompleteCompanionTrackInput struct {
	ChunkCount int              `json:"chunk_count"`
	PointCount int              `json:"point_count"`
	Track      CreateTrackInput `json:"track"`
}
type companionTrackManifest struct {
	TrackID string                      `json:"track_id"`
	Input   CompleteCompanionTrackInput `json:"input"`
}
type CompanionMemberResult struct {
	UserID     int64     `json:"user_id"`
	Nickname   string    `json:"nickname"`
	AvatarURL  string    `json:"avatar_url"`
	Color      string    `json:"color"`
	Status     string    `json:"sync_status"`
	TrackID    string    `json:"track_id,omitempty"`
	Distance   float64   `json:"distance"`
	Duration   uint32    `json:"duration"`
	PointCount int       `json:"point_count"`
	JoinedAt   time.Time `json:"joined_at"`
	LeftAt     time.Time `json:"left_at"`
}
type CompanionResult struct {
	Session *models.CompanionSession `json:"session"`
	Members []CompanionMemberResult  `json:"members"`
}
type CompanionResultTrack struct {
	*models.Track
	Nickname      string `json:"nickname"`
	UserAvatarURL string `json:"user_avatar_url"`
}
type CompanionPointPage struct {
	Points    []CompanionRecordedPoint `json:"points"`
	NextIndex int                      `json:"next_index"`
	HasMore   bool                     `json:"has_more"`
}

func (s *CompanionService) SetTrackService(tracks *TrackService) { s.trackService = tracks }

func (s *CompanionService) CanReadTrackFile(ctx context.Context, user int64, trackID string) (bool, error) {
	if s == nil || s.tracks == nil || user <= 0 {
		return false, nil
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if track.Status == models.TrackStatusDeleted || !track.DeletedAt.IsZero() {
		return false, nil
	}
	if track.UserID == user || track.Status == models.TrackStatusNormal && !track.IsRunning {
		return true, nil
	}
	if s.repo == nil || track.SessionID == "" {
		return false, nil
	}
	if _, err = s.repo.FindMember(ctx, track.SessionID, user); errors.Is(err, repository.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	storage, ok := s.repo.(repository.CompanionTrackRepository)
	if !ok {
		return false, nil
	}
	rows, err := storage.ListTrackManifests(ctx, track.SessionID)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row.UserID == track.UserID {
			var manifest companionTrackManifest
			if err = json.Unmarshal([]byte(row.Data), &manifest); err != nil {
				return false, err
			}
			return manifest.TrackID == track.ID, nil
		}
	}
	return false, nil
}

func (s *CompanionService) UpdateMemberTrackPreviews(ctx context.Context, user int64, id string, images models.TrackPreviewImages) error {
	track, err := s.MemberResultTrack(ctx, user, id, user)
	if err != nil {
		return err
	}
	_, err = s.trackService.UpdateTrackInfo(ctx, user, track.ID, TrackInfoPatch{PreviewImages: &images})
	return err
}

func (s *CompanionService) resultAccess(ctx context.Context, viewer int64, id string) (*models.CompanionSession, repository.CompanionTrackRepository, error) {
	if s == nil || s.repo == nil || s.tracks == nil || s.trackService == nil {
		return nil, nil, errors.New("companion track service unavailable")
	}
	if viewer <= 0 {
		return nil, nil, repository.ErrForbidden
	}
	session, err := s.repo.FindSessionByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if _, err = s.repo.FindMember(ctx, id, viewer); err != nil {
		return nil, nil, repository.ErrForbidden
	}
	storage, ok := s.repo.(repository.CompanionTrackRepository)
	if !ok {
		return nil, nil, errors.New("companion track storage unavailable")
	}
	return session, storage, nil
}

func (s *CompanionService) UploadTrackBatch(ctx context.Context, user int64, id string, input CompanionPointBatch) error {
	session, storage, err := s.resultAccess(ctx, user, id)
	if err != nil {
		return err
	}
	if session.Status != models.CompanionSessionStatusEnded {
		return invalidArg("companion session is not ended")
	}
	if input.Index < 1 || input.Index > 4096 || len(input.Points) == 0 || len(input.Points) > 256 {
		return invalidArg("invalid point batch")
	}
	manifests, err := storage.ListTrackManifests(ctx, id)
	if err != nil {
		return err
	}
	for _, row := range manifests {
		if row.UserID == user {
			var manifest companionTrackManifest
			if err = json.Unmarshal([]byte(row.Data), &manifest); err != nil {
				return err
			}
			if input.Index > manifest.Input.ChunkCount {
				return repository.ErrConflict
			}
		}
	}
	previous := int64(0)
	for _, p := range input.Points {
		at := time.UnixMilli(p.TimestampMillis)
		if math.IsNaN(p.Latitude) || math.IsInf(p.Latitude, 0) || p.Latitude < -85 || p.Latitude > 85 ||
			math.IsNaN(p.Longitude) || math.IsInf(p.Longitude, 0) || p.Longitude < -180 || p.Longitude > 180 ||
			math.IsNaN(p.AltitudeMeters) || math.IsInf(p.AltitudeMeters, 0) || p.SegmentID < 0 ||
			math.IsNaN(p.AccuracyMeters) || math.IsInf(p.AccuracyMeters, 0) || p.AccuracyMeters < 0 ||
			math.IsNaN(p.SpeedMps) || math.IsInf(p.SpeedMps, 0) || p.SpeedMps < 0 || math.IsNaN(p.Bearing) || math.IsInf(p.Bearing, 0) ||
			p.TimestampMillis <= previous || at.Before(session.StartedAt.Add(-time.Minute)) || at.After(session.EndedAt.Add(time.Minute)) {
			return invalidArg("invalid recorded point")
		}
		previous = p.TimestampMillis
	}
	data, err := json.Marshal(input.Points)
	if err != nil {
		return err
	}
	return storage.PutTrackChunk(ctx, models.CompanionTrackChunk{SessionID: id, UserID: user, Index: input.Index, PointCount: len(input.Points), Digest: fmt.Sprintf("%x", sha256.Sum256(data)), Data: string(data)})
}

func (s *CompanionService) CompleteMemberTrack(ctx context.Context, user int64, id string, input CompleteCompanionTrackInput) (*models.Track, error) {
	defer func() { _ = s.reconcileResultTotals(ctx, user, id) }()
	session, storage, err := s.resultAccess(ctx, user, id)
	if err != nil {
		return nil, err
	}
	if session.Status != models.CompanionSessionStatusEnded {
		return nil, invalidArg("companion session is not ended")
	}
	manifests, err := storage.ListTrackManifests(ctx, id)
	if err != nil {
		return nil, err
	}
	var manifest *companionTrackManifest
	for _, row := range manifests {
		if row.UserID == user {
			var m companionTrackManifest
			if err = json.Unmarshal([]byte(row.Data), &m); err != nil {
				return nil, err
			}
			manifest = &m
			break
		}
	}
	if manifest == nil {
		count, points, last, e := storage.TrackChunkCounts(ctx, id, user)
		if e != nil {
			return nil, e
		}
		if count != input.ChunkCount || last != count || points != input.PointCount || input.PointCount < 0 {
			return nil, invalidArg("incomplete point batches")
		}
		trackID := ""
		if points > 0 {
			if s.trackService == nil || s.tracks == nil {
				return nil, errors.New("track service unavailable")
			}
			if input.Track.RawTrackURL == nil || *input.Track.RawTrackURL == "" {
				return nil, invalidArg("raw_track_url is required")
			}
			if input.Track.StartTime == nil || input.Track.EndTime == nil || input.Track.StartTime.Before(session.StartedAt.Add(-time.Minute)) || input.Track.EndTime.After(session.EndedAt.Add(time.Minute)) {
				return nil, invalidArg("invalid recording interval")
			}
			running := false
			coordinate := "WGS84"
			source := ""
			sessionKey := id
			input.Track.IsRunning = &running
			input.Track.CoordinateSystem = &coordinate
			input.Track.TrackType = &session.TrackType
			input.Track.SessionID = &sessionKey
			input.Track.SourceTag = &source
			input.Track.normalize()
			if e = input.Track.validate(); e != nil {
				return nil, e
			}
			trackID, e = s.tracks.NextTrackID(ctx)
			if e != nil {
				return nil, e
			}
		}
		proposed := companionTrackManifest{TrackID: trackID, Input: input}
		data, e := json.Marshal(proposed)
		if e != nil {
			return nil, e
		}
		e = storage.PutTrackChunk(ctx, models.CompanionTrackChunk{SessionID: id, UserID: user, Index: 0, Digest: fmt.Sprintf("%x", sha256.Sum256(data)), Data: string(data)})
		if errors.Is(e, repository.ErrConflict) {
			return s.CompleteMemberTrack(ctx, user, id, input)
		}
		if e != nil {
			return nil, e
		}
		manifest = &proposed
	}
	if manifest.TrackID == "" {
		return nil, nil
	}
	existing, err := s.tracks.FindByID(ctx, manifest.TrackID)
	if err == nil {
		if existing.UserID != user {
			return nil, repository.ErrForbidden
		}
		return existing, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	created, err := s.trackService.createTrackAtID(ctx, user, manifest.Input.Track, manifest.TrackID, models.TrackStatusPrivate)
	if errors.Is(err, repository.ErrAlreadyExists) {
		return s.tracks.FindByID(ctx, manifest.TrackID)
	}
	return created, err
}

func (s *CompanionService) Result(ctx context.Context, viewer int64, id string) (*CompanionResult, error) {
	session, storage, err := s.resultAccess(ctx, viewer, id)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.ListMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	sort.Slice(members, func(i, j int) bool {
		if members[i].JoinedAt.Equal(members[j].JoinedAt) {
			return members[i].UserID < members[j].UserID
		}
		return members[i].JoinedAt.Before(members[j].JoinedAt)
	})
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	users, err := s.users.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	manifests, err := storage.ListTrackManifests(ctx, id)
	if err != nil {
		return nil, err
	}
	byUser := map[int64]companionTrackManifest{}
	trackIDs := []string{}
	for _, row := range manifests {
		var m companionTrackManifest
		if err = json.Unmarshal([]byte(row.Data), &m); err != nil {
			return nil, err
		}
		byUser[row.UserID] = m
		if m.TrackID != "" {
			trackIDs = append(trackIDs, m.TrackID)
		}
	}
	tracks, err := s.tracks.FindByIDs(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	colors := []string{"#E6194B", "#3CB44B", "#4363D8", "#F58231", "#911EB4", "#46F0F0", "#F032E6", "#BCF60C", "#008080", "#FFE119", "#9A6324", "#469990"}
	result := &CompanionResult{Session: companionSessionForResponse(session), Members: make([]CompanionMemberResult, 0, len(members))}
	for index, m := range members {
		row := CompanionMemberResult{UserID: m.UserID, Color: colors[index%len(colors)], Status: "syncing", JoinedAt: m.JoinedAt, LeftAt: m.LeftAt}
		if u := users[m.UserID]; u != nil {
			row.Nickname = u.Nickname
			row.AvatarURL = fallbackAvatarURL(u.ID, u.AvatarURL)
			if s.avatarCache != nil && shouldRewriteAvatarURL(row.AvatarURL) {
				source := row.AvatarURL
				key := formatUserAvatarCacheKey(u.ID)
				s.avatarCache.PrefetchAsync(u.ID, key, source)
				row.AvatarURL = s.avatarCache.GuessLocalURL(key, source)
			}
		}
		if manifest, ok := byUser[m.UserID]; ok {
			row.PointCount = manifest.Input.PointCount
			if manifest.TrackID == "" {
				row.Status = "empty"
			} else if t := tracks[manifest.TrackID]; t != nil {
				if t.Status == models.TrackStatusDeleted || !t.DeletedAt.IsZero() {
					row.Status = "unavailable"
				} else {
					row.Status = "ready"
					row.TrackID = t.ID
					row.Distance = t.Distance
					row.Duration = t.Duration
				}
			}
		}
		result.Members = append(result.Members, row)
	}
	return result, nil
}

func (s *CompanionService) MemberResultTrack(ctx context.Context, viewer int64, id string, member int64) (*CompanionResultTrack, error) {
	_, storage, err := s.resultAccess(ctx, viewer, id)
	if err != nil {
		return nil, err
	}
	manifests, err := storage.ListTrackManifests(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, row := range manifests {
		if row.UserID == member {
			var m companionTrackManifest
			if err = json.Unmarshal([]byte(row.Data), &m); err != nil {
				return nil, err
			}
			if m.TrackID == "" {
				return nil, repository.ErrNotFound
			}
			track, err := s.trackService.GetTrackDetail(ctx, m.TrackID)
			if err != nil {
				return nil, err
			}
			if track.UserID != member || track.Status == models.TrackStatusDeleted || !track.DeletedAt.IsZero() {
				return nil, repository.ErrNotFound
			}
			if err = s.trackService.FillTrackLikeState(ctx, viewer, track); err != nil {
				return nil, err
			}
			if err = s.trackService.FillTrackCollectedState(ctx, viewer, track); err != nil {
				return nil, err
			}
			result := &CompanionResultTrack{Track: track}
			if author, e := s.users.FindByID(ctx, member); e == nil && author != nil {
				result.Nickname = author.Nickname
				result.UserAvatarURL = fallbackAvatarURL(author.ID, author.AvatarURL)
				if s.avatarCache != nil && shouldRewriteAvatarURL(result.UserAvatarURL) {
					key := formatUserAvatarCacheKey(author.ID)
					if ext, ok := s.avatarCache.Exists(key); ok {
						result.UserAvatarURL = s.avatarCache.LocalURL(key, ext)
					}
				}
			}
			if track.Status == models.TrackStatusPrivate && viewer != member {
				track.CanComment = false
			}
			return result, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (s *CompanionService) ResultPoints(ctx context.Context, viewer int64, id string, member int64, after int) (*CompanionPointPage, error) {
	_, storage, err := s.resultAccess(ctx, viewer, id)
	if err != nil {
		return nil, err
	}
	if _, err = s.MemberResultTrack(ctx, viewer, id, member); err != nil {
		return nil, err
	}
	if after < 0 {
		return nil, invalidArg("invalid cursor")
	}
	manifests, err := storage.ListTrackManifests(ctx, id)
	if err != nil {
		return nil, err
	}
	last := 0
	for _, row := range manifests {
		if row.UserID == member {
			var manifest companionTrackManifest
			if err = json.Unmarshal([]byte(row.Data), &manifest); err != nil {
				return nil, err
			}
			last = manifest.Input.ChunkCount
			break
		}
	}
	if after >= last {
		return &CompanionPointPage{Points: []CompanionRecordedPoint{}, NextIndex: after}, nil
	}
	chunks, err := storage.ListTrackChunks(ctx, id, member, after, 17)
	if err != nil {
		return nil, err
	}
	page := &CompanionPointPage{Points: []CompanionRecordedPoint{}, HasMore: len(chunks) > 16, NextIndex: after}
	if len(chunks) > 16 {
		chunks = chunks[:16]
	}
	for _, c := range chunks {
		if c.Index > last {
			break
		}
		var points []CompanionRecordedPoint
		if err = json.Unmarshal([]byte(c.Data), &points); err != nil {
			return nil, err
		}
		page.Points = append(page.Points, points...)
		page.NextIndex = c.Index
	}
	page.HasMore = page.NextIndex < last
	return page, nil
}

func (s *CompanionService) authoritativeResultTotals(ctx context.Context, user int64, session *models.CompanionSession) (bool, error) {
	if s.trackService == nil {
		return false, nil
	}
	result, err := s.Result(ctx, user, session.SessionID)
	if err != nil {
		return false, err
	}
	distance := 0.0
	for _, member := range result.Members {
		if member.Status == "syncing" {
			return false, nil
		}
		distance += member.Distance
	}
	session.TotalDistance = distance
	session.TotalDuration = int64(session.EndedAt.Sub(session.StartedAt).Seconds())
	if session.TotalDuration < 0 {
		session.TotalDuration = 0
	}
	session.ActualParticipantCount = int64(len(result.Members))
	return true, nil
}

func (s *CompanionService) reconcileResultTotals(ctx context.Context, user int64, id string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	session, err := s.repo.FindSessionByID(ctx, id)
	if err != nil {
		return err
	}
	ready, err := s.authoritativeResultTotals(ctx, user, session)
	if err != nil || !ready {
		return err
	}
	storage, ok := s.repo.(repository.CompanionTrackRepository)
	if !ok {
		return nil
	}
	return storage.SetResultTotals(ctx, session.SessionID, session.TotalDistance, session.TotalDuration, session.ActualParticipantCount)
}
