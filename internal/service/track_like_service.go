package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type TrackLikeStatus struct {
	models.TrackLikeStats
	CanLike bool `json:"can_like"`
}
type TrackLikeService struct {
	likes  repository.TrackLikeRepository
	tracks repository.TrackRepository
}

func NewTrackLikeService(likes repository.TrackLikeRepository, tracks repository.TrackRepository) *TrackLikeService {
	return &TrackLikeService{likes: likes, tracks: tracks}
}
func likeEligible(track *models.Track) bool {
	return track != nil && track.Status == models.TrackStatusNormal && !track.IsRunning && strings.TrimSpace(track.RawTrackURL) != ""
}
func (s *TrackLikeService) Status(ctx context.Context, viewer int64, trackID string) (*TrackLikeStatus, error) {
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if !likeEligible(track) {
		if track.UserID == viewer && track.Status != models.TrackStatusDeleted {
			return &TrackLikeStatus{}, nil
		}
		return nil, repository.ErrNotFound
	}
	stats, err := s.likes.Stats(ctx, viewer, []string{trackID})
	if err != nil {
		return nil, err
	}
	return &TrackLikeStatus{TrackLikeStats: stats[trackID], CanLike: true}, nil
}
func (s *TrackLikeService) Set(ctx context.Context, viewer int64, trackID string, liked bool) (*TrackLikeStatus, error) {
	if viewer <= 0 || strings.TrimSpace(trackID) == "" {
		return nil, invalidArg("user_id and track_id are required")
	}
	if liked {
		track, err := s.tracks.FindByID(ctx, trackID)
		if err != nil {
			return nil, err
		}
		if !likeEligible(track) {
			return nil, repository.ErrNotFound
		}
	}
	if err := s.likes.SetLike(ctx, viewer, trackID, liked); err != nil {
		return nil, err
	}
	status, err := s.Status(ctx, viewer, trackID)
	if !liked && errors.Is(err, repository.ErrNotFound) {
		return &TrackLikeStatus{}, nil
	}
	return status, err
}
func (s *TrackService) SetTrackLikeRepository(likes repository.TrackLikeRepository) { s.likes = likes }
func (s *TrackService) trackLikeStats(ctx context.Context, viewer int64, ids []string) (map[string]models.TrackLikeStats, error) {
	if s.likes == nil {
		return map[string]models.TrackLikeStats{}, nil
	}
	return s.likes.Stats(ctx, viewer, ids)
}
func (s *TrackService) FillTrackLikeState(ctx context.Context, viewer int64, track *models.Track) error {
	if track == nil || track.Status != models.TrackStatusNormal || track.IsRunning {
		return nil
	}
	if strings.TrimSpace(track.RawTrackURL) == "" {
		// Asset cache failures clear the response URL without making the stored route unpublished.
		stored, err := s.tracks.FindByID(ctx, track.ID)
		if err != nil {
			return err
		}
		if !likeEligible(stored) {
			return nil
		}
	}
	stats, err := s.trackLikeStats(ctx, viewer, []string{track.ID})
	if err != nil {
		return err
	}
	track.Liked = stats[track.ID].Liked
	track.LikeCount = stats[track.ID].LikeCount
	return nil
}
func (s *UserService) SetTrackLikeRepository(likes repository.TrackLikeRepository) { s.likes = likes }

// ListLikedTracks 返回「当前用户点赞过的轨迹列表」。
//
// 与 ListCollectedTracks 完全同构：按点赞 created_at desc, track_id desc 游标翻页，
// 剔除删除/私密/进行中轨迹，响应结构复用 CollectedTrackSummaryPage。
func (s *TrackService) ListLikedTracks(ctx context.Context, userID int64, input ListCollectedTracksInput) (*models.LikedTrackSummaryPage, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	if s.likes == nil {
		return nil, errors.New("track like repository not configured")
	}
	limit := normalizeTrackPageLimit(input.Limit)
	cur, err := decodeTrackCollectCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	totalCount, err := s.countVisibleLikedTotalCount(ctx, userID)
	if err != nil {
		return nil, err
	}

	batch := limit + 1
	recs, err := s.likes.ListLikedByUser(ctx, userID, cur, batch)
	if err != nil {
		return nil, err
	}

	tracks := make([]*models.Track, 0, len(recs))
	includedCursors := make([]models.TrackLikeCursor, 0, len(recs))
	for _, rec := range recs {
		t, err := s.tracks.FindByID(ctx, rec.TrackID)
		switch {
		case err == nil && t != nil:
			// 与收藏列表一致：排除删除/私密/进行中轨迹，兜底保护。
			if t.Status != models.TrackStatusNormal || t.IsRunning {
				continue
			}
			tracks = append(tracks, t)
			includedCursors = append(includedCursors, models.TrackLikeCursor{CreatedAt: rec.CreatedAt, TrackID: rec.TrackID})
		case errors.Is(err, repository.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		}
	}

	hasMore := len(tracks) > limit
	var nextCursorSrc *models.TrackLikeCursor
	if hasMore {
		c := includedCursors[limit-1]
		nextCursorSrc = &c
		tracks = tracks[:limit]
	} else if len(tracks) > 0 {
		c := includedCursors[len(includedCursors)-1]
		nextCursorSrc = &c
	}

	summaries := toSummaries(tracks)
	if s.screenshotCache != nil || s.rawTrackCache != nil {
		for i, t := range tracks {
			cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if s.screenshotCache != nil {
				summaries[i].TrackScreenshotURL = s.screenshotCache.EnsureCached(cacheCtx, userID, t.ID, t.TrackScreenshotURL)
				if t.TrackNoMapBgScreenshotURL != "" {
					summaries[i].TrackNoMapBgScreenshotURL = s.screenshotCache.EnsureCached(cacheCtx, userID, t.ID+"_no_map_bg", t.TrackNoMapBgScreenshotURL)
				}
			}
			if s.rawTrackCache != nil {
				summaries[i].RawTrackURL = s.rawTrackCache.EnsureCached(cacheCtx, userID, t.ID, t.RawTrackURL)
			}
			cancel()
		}
	}
	if err := s.fillTrackSummaryExtras(ctx, userID, summaries); err != nil {
		return nil, err
	}

	items := make([]*models.CollectedTrackSummary, 0, len(summaries))
	if s.submissions != nil && len(summaries) > 0 {
		trackIDs := make([]string, 0, len(summaries))
		for _, summary := range summaries {
			trackIDs = append(trackIDs, summary.ID)
		}
		approved, err := s.submissions.ApprovedTrackIDs(ctx, trackIDs)
		if err != nil {
			return nil, err
		}
		for _, summary := range summaries {
			summary.IsFeatured = approved[summary.ID]
			summary.Labels = buildTrackLabels(summary.ShapeLabel, summary.TrackType, summary.PrimaryScenicSpot, summary.IsFeatured)
		}
	}
	for _, s := range summaries {
		items = append(items, &models.CollectedTrackSummary{
			Labels:            s.Labels,
			ShapeLabel:        s.ShapeLabel,
			PrimaryScenicSpot: s.PrimaryScenicSpot,
			IsFeatured:        s.IsFeatured,
			CommentCount:      s.CommentCount, CanComment: s.CanComment,
			Liked: s.Liked, LikeCount: s.LikeCount,
			ID:                        s.ID,
			UserID:                    s.UserID,
			SessionID:                 s.SessionID,
			CityCode:                  s.CityCode,
			LocateAddr:                s.LocateAddr,
			TrackType:                 s.TrackType,
			StartTime:                 s.StartTime,
			EndTime:                   s.EndTime,
			CityName:                  s.CityName,
			Nickname:                  s.Nickname,
			UserAvatarURL:             s.UserAvatarURL,
			Title:                     s.Title,
			Distance:                  s.Distance,
			Duration:                  s.Duration,
			AvgSpeedKmh:               s.AvgSpeedKmh,
			CaloriesBurned:            s.CaloriesBurned,
			ElevationGain:             s.ElevationGain,
			Collected:                 s.Collected,
			CollectCount:              s.CollectCount,
			NavigateCount:             s.NavigateCount,
			TrackScreenshotURL:        s.TrackScreenshotURL,
			PreviewImages:             s.PreviewImages,
			TrackNoMapBgScreenshotURL: s.TrackNoMapBgScreenshotURL,
			RawTrackURL:               s.RawTrackURL,
		})
	}

	page := &models.LikedTrackSummaryPage{Items: items, HasMore: hasMore, TotalCount: totalCount}
	if hasMore && nextCursorSrc != nil {
		nextCursor, err := encodeTrackCollectCursor(nextCursorSrc.CreatedAt, nextCursorSrc.TrackID)
		if err != nil {
			return nil, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

// countVisibleLikedTotalCount 统计用户点赞记录中当前可见（正常且非进行中）的轨迹总数。
// 逐页扫描点赞记录 + FindByID 过滤，与收藏总数的 fallback 口径一致。
func (s *TrackService) countVisibleLikedTotalCount(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 || s.likes == nil || s.tracks == nil {
		return 0, nil
	}
	var (
		cursor *models.TrackLikeCursor
		limit  = 200
		count  int64
	)
	for page := 0; page < 10000; page++ { // guard
		likes, err := s.likes.ListLikedByUser(ctx, userID, cursor, limit)
		if err != nil {
			return 0, err
		}
		if len(likes) == 0 {
			break
		}
		for _, l := range likes {
			if l == nil || l.TrackID == "" {
				continue
			}
			t, err := s.tracks.FindByID(ctx, l.TrackID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}
				return 0, err
			}
			if t != nil && t.Status == models.TrackStatusNormal && !t.IsRunning {
				count++
			}
		}
		last := likes[len(likes)-1]
		if last == nil || last.TrackID == "" || last.CreatedAt.IsZero() {
			break
		}
		cursor = &models.TrackLikeCursor{CreatedAt: last.CreatedAt, TrackID: last.TrackID}
		if len(likes) < limit {
			break
		}
	}
	return count, nil
}
