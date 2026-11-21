package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

type AccountDeletionService struct {
	store         *repository.MySQLAccountDeletionStore
	oss           *OSSTokenService
	tracks        *TrackService
	background    *AssetCacheService
	submissions   *AssetCacheService
	comments      *AssetCacheService
	feedbackRoot  string
	testMode      bool
	wechatEnabled bool
	gracePeriod   time.Duration
}

func NewAccountDeletionService(store *repository.MySQLAccountDeletionStore, oss *OSSTokenService, tracks *TrackService, background, submissions, comments *AssetCacheService, feedbackRoot string, testMode, wechatEnabled bool, mqttTTL time.Duration) *AccountDeletionService {
	if store == nil || oss == nil || oss.ossInternalEndpoint == "" {
		return nil
	}
	gracePeriod := max(time.Duration(oss.durationSeconds)*time.Second, mqttTTL) + time.Minute
	return &AccountDeletionService{store: store, oss: oss, tracks: tracks, background: background, submissions: submissions, comments: comments,
		feedbackRoot: feedbackRoot, testMode: testMode, wechatEnabled: wechatEnabled, gracePeriod: gracePeriod}
}

func (s *AccountDeletionService) Enabled() bool {
	return s != nil && (s.testMode || s.wechatEnabled)
}

func (s *AccountDeletionService) CanConfirm(wechatVerified bool) bool {
	return s.testMode || (s.wechatEnabled && wechatVerified)
}

func (s *AccountDeletionService) AvailableFor(ctx context.Context, userID int64, wechatVerified bool) bool {
	return s.CanConfirm(wechatVerified) || (s.wechatEnabled && s.store.HasWechatLogin(ctx, userID))
}

func (s *AccountDeletionService) Request(ctx context.Context, userID int64) error {
	now := time.Now().UTC()
	// Give in-flight API requests time to finish before deleting rows. OSS remains writable until its issued credentials expire.
	return s.store.Request(ctx, userID, now, now.Add(time.Minute), now.Add(s.gracePeriod))
}

func (s *AccountDeletionService) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := s.runNext(attemptCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Printf("account deletion worker: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *AccountDeletionService) runNext(ctx context.Context) error {
	job, err := s.store.Claim(ctx, time.Now().UTC(), 5*time.Minute)
	if err != nil {
		return fmt.Errorf("claim deletion: %w", err)
	}
	if job == nil {
		return nil
	}
	objectKey := func(source string) string {
		return s.objectKey(job.UserID, source)
	}
	assets, err := s.store.PurgeData(ctx, *job, objectKey)
	if err != nil {
		return fmt.Errorf("purge account records: %w", err)
	}
	if err := s.removeLocalFiles(job.UserID, assets); err != nil {
		return err
	}
	if len(assets.Objects) > 0 && time.Now().Before(job.ObjectsAfter) {
		return s.store.WaitForObjectCredentials(ctx, *job)
	}
	if len(assets.Objects) > 0 {
		assets.Objects, err = s.store.UnsharedObjects(ctx, job.UserID, assets.Objects, objectKey)
		if err != nil {
			return fmt.Errorf("check account OSS object references: %w", err)
		}
	}
	for len(assets.Objects) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Reserve room for JSON escaping and the STS policy envelope.
		count, budget := 0, 1350
		for _, key := range assets.Objects {
			if objectKey("/"+key) != key {
				return fmt.Errorf("invalid object in account deletion manifest")
			}
			budget -= len(key) + len(s.oss.ossBucket) + 22
			if count > 0 && budget < 0 {
				break
			}
			count++
		}
		if err := s.oss.deleteExactObjects(ctx, assets.Objects[:count]); err != nil {
			return fmt.Errorf("delete account OSS objects: %w", err)
		}
		assets.Objects = assets.Objects[count:]
		if err := s.store.SaveAssets(ctx, *job, assets); err != nil {
			return fmt.Errorf("save account deletion progress: %w", err)
		}
	}
	return s.store.Finish(ctx, *job, time.Now().UTC())
}

func (s *AccountDeletionService) objectKey(userID int64, source string) string {
	u, err := url.Parse(source)
	if err != nil || u.User != nil || u.Opaque != "" {
		return ""
	}
	if u.Host != "" || u.Scheme != "" {
		if (u.Scheme != "https" && u.Scheme != "http") || u.Port() != "" || !s.managedOSSHost(u.Hostname()) {
			return ""
		}
	} else if u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	key := strings.TrimPrefix(u.Path, "/")
	prefix := fmt.Sprintf("%s/%d/", strings.Trim(s.oss.ossUploadPrefix, "/"), GetBucketID(userID))
	if !strings.HasPrefix(key, prefix) || len(key) > 1023 || path.Clean(key) != key || strings.ContainsAny(key, "*?\\#%\"<>&") {
		return ""
	}
	for _, ch := range key {
		if ch < 32 || ch > 126 {
			return ""
		}
	}
	return key
}

func (s *AccountDeletionService) managedOSSHost(host string) bool {
	for _, endpoint := range []string{s.oss.ossEndpoint, s.oss.ossInternalEndpoint} {
		if !strings.Contains(endpoint, "://") {
			endpoint = "https://" + endpoint
		}
		u, err := url.Parse(endpoint)
		if err == nil && u.Hostname() != "" && strings.EqualFold(host, s.oss.ossBucket+"."+u.Hostname()) {
			return true
		}
	}
	return false
}

func (s *AccountDeletionService) removeLocalFiles(userID int64, assets repository.AccountDeletionAssets) error {
	remove := func(cache *AssetCacheService, keys ...string) error {
		for _, key := range keys {
			if cache != nil && key != "" && key != "." && key != ".." && filepath.Base(key) == key {
				if err := cache.RemoveCached(key); err != nil {
					return fmt.Errorf("remove account asset cache: %w", err)
				}
			}
		}
		return nil
	}
	uid := strconv.FormatInt(userID, 10)
	if s.background != nil {
		entries, err := os.ReadDir(s.background.cacheDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("list profile background cache: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), uid+"_") {
				err := os.Remove(filepath.Join(s.background.cacheDir, entry.Name()))
				if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("remove profile background cache: %w", err)
				}
			}
		}
	}
	if err := remove(s.tracks.avatarCache, uid); err != nil {
		return err
	}
	for _, track := range assets.Tracks {
		if err := remove(s.tracks.rawTrackCache, track.TrackID); err != nil {
			return err
		}
		if err := remove(s.tracks.screenshotCache, track.TrackID, track.TrackID+"_no_map_bg", track.TrackID+"_landscape_day"); err != nil {
			return err
		}
		for _, variant := range previewVariants {
			if source := track.Preview[variant+"_url"]; source != "" {
				key := TrackPreviewCacheKey(track.TrackID, variant, source)
				if err := remove(s.tracks.previewCache, key); err != nil {
					return err
				}
				if variant == "landscape_day" {
					if err := remove(s.tracks.screenshotCache, key); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, cover := range assets.CompanionCovers {
		key := companionScreenshotCacheKey(cover.SessionID)
		if err := remove(s.tracks.screenshotCache, key); err != nil {
			return err
		}
		if !companionCoverVersionPattern.MatchString(cover.Version) {
			continue
		}
		for _, variant := range previewVariants {
			if err := remove(s.tracks.screenshotCache, key+"_preview_"+cover.Version+"_"+variant); err != nil {
				return err
			}
		}
	}
	if err := remove(s.submissions, assets.SubmissionImageIDs...); err != nil {
		return err
	}
	if err := remove(s.comments, assets.CommentImageIDs...); err != nil {
		return err
	}
	if s.feedbackRoot != "" {
		if err := os.RemoveAll(filepath.Join(s.feedbackRoot, uid)); err != nil {
			return fmt.Errorf("remove account feedback attachments: %w", err)
		}
	}
	return nil
}
