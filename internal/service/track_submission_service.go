package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

var (
	difficultyOptions = map[string]string{"easy": "轻松", "standard": "标准", "hard": "困难", "challenge": "挑战", "extreme": "极限"}
	riskOptions       = map[string]string{"none": "无风险", "low": "低风险", "medium": "中风险", "high": "高风险"}
	surfaceOptions    = map[string]string{"road": "公路", "boardwalk": "栈道", "stone_slab": "石板路", "stairs": "台阶", "dirt": "土路", "wading": "涉水", "loose_rocks": "乱石", "climbing": "攀岩", "shrub": "灌木丛", "snow": "雪地", "desert": "沙漠", "reef": "礁石"}
	transportOptions  = map[string]string{"bus": "公交", "metro": "地铁", "train": "火车", "self_drive": "自驾", "taxi": "出租车/网约车", "chartered": "包车", "ferry": "轮渡", "cable_car": "缆车", "cycling": "骑行", "walking": "步行", "other": "其他"}
)

type TrackSubmissionImageInput struct {
	OSSURL    string `json:"oss_url"`
	Caption   string `json:"caption"`
	SortOrder int    `json:"sort_order"`
}

type TrackSubmissionInput struct {
	Title                string                      `json:"title"`
	Description          string                      `json:"description"`
	Difficulty           string                      `json:"difficulty"`
	RiskLevel            string                      `json:"risk_level"`
	SuitableMonths       []int                       `json:"suitable_months"`
	SurfaceTypes         []string                    `json:"surface_types"`
	TransportModes       []string                    `json:"transport_modes"`
	TransportDescription string                      `json:"transport_description"`
	Images               []TrackSubmissionImageInput `json:"images"`
}

type TrackSubmissionOption struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type TrackSubmissionOptions struct {
	DifficultyOptions []TrackSubmissionOption `json:"difficulty_options"`
	RiskLevelOptions  []TrackSubmissionOption `json:"risk_level_options"`
	SurfaceOptions    []TrackSubmissionOption `json:"surface_type_options"`
	TransportOptions  []TrackSubmissionOption `json:"transport_mode_options"`
	ImageLimits       map[string]int          `json:"image_limits"`
}

type TrackSubmissionService struct {
	repo       repository.TrackSubmissionRepository
	tracks     repository.TrackRepository
	imageCache *AssetCacheService
}

func NewTrackSubmissionService(repo repository.TrackSubmissionRepository, tracks repository.TrackRepository) *TrackSubmissionService {
	return &TrackSubmissionService{repo: repo, tracks: tracks}
}

func (s *TrackSubmissionService) SetImageCache(cache *AssetCacheService) { s.imageCache = cache }

func submissionID(prefix string) (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + strings.ToUpper(hex.EncodeToString(buf)), nil
}

func sortedOptions(values map[string]string, descriptions map[string]string) []TrackSubmissionOption {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]TrackSubmissionOption, 0, len(keys))
	for _, key := range keys {
		result = append(result, TrackSubmissionOption{Code: key, Name: values[key], Description: descriptions[key]})
	}
	return result
}

func (s *TrackSubmissionService) Options() TrackSubmissionOptions {
	difficulties := map[string]string{"easy": "路线平缓不陡，适合新手", "standard": "有小幅起伏，需要轻度体力", "hard": "爬升较多、路况复杂，需要一定经验", "challenge": "高海拔或长距离，部分路段险峻", "extreme": "极端环境、超高强度，需要专业装备和经验"}
	risks := map[string]string{"none": "路面平坦，未识别明显危险因素", "low": "存在小坡度或碎石，需要注意脚下", "medium": "存在陡坡险路，受天气影响较大", "high": "可能存在悬崖峭壁或恶劣天气等高风险因素"}
	return TrackSubmissionOptions{DifficultyOptions: sortedOptions(difficultyOptions, difficulties), RiskLevelOptions: sortedOptions(riskOptions, risks), SurfaceOptions: sortedOptions(surfaceOptions, nil), TransportOptions: sortedOptions(transportOptions, nil), ImageLimits: map[string]int{"min_count": 0, "max_count": 9, "max_file_size": 10 * 1024 * 1024, "max_total_size": 50 * 1024 * 1024}}
}

func normalizeStringSet(values []string, allowed map[string]string, field string) ([]string, error) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if _, ok := allowed[value]; !ok {
			return nil, invalidArg("invalid " + field)
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return nil, invalidArg(field + " is required")
	}
	sort.Strings(result)
	return result, nil
}

func validateSubmissionOSSURL(raw string, userID int64) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return invalidArg("invalid submission image oss_url")
	}
	ownerSegment := "/" + strconv.FormatInt(userID%config.OSSFileBucketSize, 10) + "/submission/" + strconv.FormatInt(userID, 10) + "/"
	if !strings.Contains(u.EscapedPath(), ownerSegment) {
		return invalidArg("submission image does not belong to current user")
	}
	return nil
}

func normalizeSubmissionInput(input TrackSubmissionInput, userID int64) (TrackSubmissionInput, error) {
	input.Title, input.Description = strings.TrimSpace(input.Title), strings.TrimSpace(input.Description)
	input.Difficulty, input.RiskLevel = strings.TrimSpace(input.Difficulty), strings.TrimSpace(input.RiskLevel)
	input.TransportDescription = strings.TrimSpace(input.TransportDescription)
	if n := utf8.RuneCountInString(input.Title); n < 4 || n > 40 {
		return input, invalidArg("submission title length must be between 4 and 40")
	}
	if n := utf8.RuneCountInString(input.Description); n < 20 || n > 500 {
		return input, invalidArg("submission description length must be between 20 and 500")
	}
	if _, ok := difficultyOptions[input.Difficulty]; !ok {
		return input, invalidArg("invalid difficulty")
	}
	if _, ok := riskOptions[input.RiskLevel]; !ok {
		return input, invalidArg("invalid risk_level")
	}
	monthSet := make(map[int]bool)
	for _, month := range input.SuitableMonths {
		if month < 1 || month > 12 {
			return input, invalidArg("invalid suitable_months")
		}
		monthSet[month] = true
	}
	if len(monthSet) == 0 {
		return input, invalidArg("at least one suitable month is required")
	}
	input.SuitableMonths = input.SuitableMonths[:0]
	for month := range monthSet {
		input.SuitableMonths = append(input.SuitableMonths, month)
	}
	sort.Ints(input.SuitableMonths)
	var err error
	if input.SurfaceTypes, err = normalizeStringSet(input.SurfaceTypes, surfaceOptions, "surface_types"); err != nil {
		return input, err
	}
	if input.TransportModes, err = normalizeStringSet(input.TransportModes, transportOptions, "transport_modes"); err != nil {
		return input, err
	}
	if n := utf8.RuneCountInString(input.TransportDescription); n < 10 || n > 500 {
		return input, invalidArg("transport_description length must be between 10 and 500")
	}
	if len(input.Images) > 9 {
		return input, invalidArg("submission image count must not exceed 9")
	}
	orders := make(map[int]bool)
	for i := range input.Images {
		input.Images[i].OSSURL, input.Images[i].Caption = strings.TrimSpace(input.Images[i].OSSURL), strings.TrimSpace(input.Images[i].Caption)
		if err := validateSubmissionOSSURL(input.Images[i].OSSURL, userID); err != nil {
			return input, err
		}
		if utf8.RuneCountInString(input.Images[i].Caption) > 100 || input.Images[i].SortOrder < 1 || orders[input.Images[i].SortOrder] {
			return input, invalidArg("invalid submission image")
		}
		orders[input.Images[i].SortOrder] = true
	}
	sort.Slice(input.Images, func(i, j int) bool { return input.Images[i].SortOrder < input.Images[j].SortOrder })
	return input, nil
}

func snapshotSubmission(sub *models.TrackSubmission) string {
	buf, _ := json.Marshal(sub)
	return string(buf)
}

func submissionEvent(sub *models.TrackSubmission, eventType string, from, to models.TrackSubmissionStatus, operatorType, operator, reason string, now time.Time) *models.TrackSubmissionEvent {
	return &models.TrackSubmissionEvent{SubmissionID: sub.SubmissionID, Revision: sub.Revision, EventType: eventType, FromStatus: from, ToStatus: to, OperatorType: operatorType, Operator: operator, Reason: reason, SnapshotJSON: snapshotSubmission(sub), CreatedAt: now}
}

func (s *TrackSubmissionService) Submit(ctx context.Context, userID int64, trackID string, input TrackSubmissionInput, editingPending bool) (*models.TrackSubmission, error) {
	if s == nil || s.repo == nil || s.tracks == nil {
		return nil, errors.New("track submission service is not configured")
	}
	track, err := s.tracks.FindByID(ctx, strings.TrimSpace(trackID))
	if err != nil {
		return nil, err
	}
	if track.UserID != userID {
		return nil, ErrForbidden
	}
	if track.IsRunning || track.Status != models.TrackStatusNormal || strings.TrimSpace(track.RawTrackURL) == "" || strings.TrimSpace(track.TrackScreenshotURL) == "" {
		return nil, invalidArg("track is not eligible for submission")
	}
	input, err = normalizeSubmissionInput(input, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	old, findErr := s.repo.FindByTrackID(ctx, track.ID)
	if findErr != nil && !errors.Is(findErr, repository.ErrNotFound) {
		return nil, findErr
	}
	if editingPending && old == nil {
		return nil, repository.ErrNotFound
	}
	if old != nil {
		if editingPending && old.Status != models.TrackSubmissionStatusPending {
			return nil, repository.ErrAlreadyExists
		}
		if !editingPending && old.Status != models.TrackSubmissionStatusRejected && old.Status != models.TrackSubmissionStatusWithdrawn && old.Status != models.TrackSubmissionStatusInvalidated {
			return nil, repository.ErrAlreadyExists
		}
	}
	id := ""
	createdAt := now
	revision := int64(1)
	fromStatus := models.TrackSubmissionStatus("")
	if old != nil {
		id, createdAt, revision, fromStatus = old.SubmissionID, old.CreatedAt, old.Revision+1, old.Status
	} else if id, err = submissionID("TS."); err != nil {
		return nil, err
	}
	sub := &models.TrackSubmission{SubmissionID: id, TrackID: track.ID, UserID: userID, TrackType: normalizeTrackTypeCode(track.TrackType), Title: input.Title, Description: input.Description, Difficulty: input.Difficulty, RiskLevel: input.RiskLevel, SuitableMonths: input.SuitableMonths, SurfaceTypes: input.SurfaceTypes, TransportModes: input.TransportModes, TransportDescription: input.TransportDescription, Status: models.TrackSubmissionStatusPending, Revision: revision, SubmittedAt: now, CreatedAt: createdAt, UpdatedAt: now, Images: make([]*models.TrackSubmissionImage, 0, len(input.Images))}
	for _, item := range input.Images {
		imageID, err := submissionID("TSI.")
		if err != nil {
			return nil, err
		}
		sub.Images = append(sub.Images, &models.TrackSubmissionImage{ImageID: imageID, SubmissionID: id, OSSURL: item.OSSURL, Caption: item.Caption, SortOrder: item.SortOrder, CreatedAt: now, UpdatedAt: now})
	}
	eventType := "submitted"
	if old != nil {
		eventType = "resubmitted"
	}
	if err := s.repo.SavePending(ctx, sub, submissionEvent(sub, eventType, fromStatus, sub.Status, "user", strconv.FormatInt(userID, 10), "", now)); err != nil {
		return nil, err
	}
	s.prefetchImages(sub)
	s.decorateImages(ctx, sub)
	return sub, nil
}

func (s *TrackSubmissionService) Get(ctx context.Context, viewerUserID int64, trackID string) (*models.TrackSubmission, error) {
	sub, err := s.repo.FindByTrackID(ctx, strings.TrimSpace(trackID))
	if err != nil {
		return nil, err
	}
	if sub.UserID != viewerUserID && sub.Status != models.TrackSubmissionStatusApproved {
		return nil, repository.ErrNotFound
	}
	if sub.UserID != viewerUserID {
		sub.ReviewedBy, sub.ReviewReason = "", ""
	}
	s.decorateImages(ctx, sub)
	return sub, nil
}

func (s *TrackSubmissionService) Withdraw(ctx context.Context, userID int64, trackID string) error {
	sub, err := s.repo.FindByTrackID(ctx, strings.TrimSpace(trackID))
	if err != nil {
		return err
	}
	now := time.Now()
	event := submissionEvent(sub, "withdrawn", sub.Status, models.TrackSubmissionStatusWithdrawn, "user", strconv.FormatInt(userID, 10), "", now)
	return s.repo.Withdraw(ctx, sub.TrackID, userID, now, event)
}

func (s *TrackSubmissionService) Invalidate(ctx context.Context, trackID, reason string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	sub, err := s.repo.FindByTrackID(ctx, trackID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil || sub.Status != models.TrackSubmissionStatusApproved {
		return err
	}
	now := time.Now()
	event := submissionEvent(sub, "invalidated", sub.Status, models.TrackSubmissionStatusInvalidated, "system", "track_service", reason, now)
	return s.repo.Invalidate(ctx, trackID, reason, now, event)
}

func (s *TrackSubmissionService) Review(ctx context.Context, submissionID string, expectedRevision int64, decision, reviewer, reason string) (*models.TrackSubmission, error) {
	sub, err := s.repo.FindBySubmissionID(ctx, strings.TrimSpace(submissionID))
	if err != nil {
		return nil, err
	}
	var status models.TrackSubmissionStatus
	switch decision {
	case "approved":
		status = models.TrackSubmissionStatusApproved
		track, trackErr := s.tracks.FindByID(ctx, sub.TrackID)
		if trackErr != nil {
			return nil, trackErr
		}
		if track.IsRunning || track.Status != models.TrackStatusNormal || strings.TrimSpace(track.RawTrackURL) == "" || strings.TrimSpace(track.TrackScreenshotURL) == "" {
			return nil, invalidArg("track is not eligible for submission approval")
		}
	case "rejected":
		status = models.TrackSubmissionStatusRejected
		if strings.TrimSpace(reason) == "" {
			return nil, invalidArg("review reason is required")
		}
	default:
		return nil, invalidArg("invalid review decision")
	}
	now := time.Now()
	event := submissionEvent(sub, decision, sub.Status, status, "admin", reviewer, strings.TrimSpace(reason), now)
	if err := s.repo.Review(ctx, sub.SubmissionID, expectedRevision, status, reviewer, strings.TrimSpace(reason), now, event); err != nil {
		return nil, err
	}
	return s.repo.FindBySubmissionID(ctx, sub.SubmissionID)
}

func (s *TrackSubmissionService) ListAdmin(ctx context.Context, filter models.TrackSubmissionListFilter) ([]*models.TrackSubmission, error) {
	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *TrackSubmissionService) GetAdmin(ctx context.Context, submissionID string) (*models.TrackSubmission, error) {
	sub, err := s.repo.FindBySubmissionID(ctx, strings.TrimSpace(submissionID))
	if err != nil {
		return nil, err
	}
	s.decorateImages(ctx, sub)
	return sub, nil
}

func (s *TrackSubmissionService) Events(ctx context.Context, submissionID string) ([]*models.TrackSubmissionEvent, error) {
	return s.repo.ListEvents(ctx, submissionID)
}

func (s *TrackSubmissionService) prefetchImages(sub *models.TrackSubmission) {
	if s.imageCache == nil || sub == nil {
		return
	}
	for _, image := range sub.Images {
		if image != nil {
			s.imageCache.PrefetchAsync(sub.UserID, sub.SubmissionID+"_"+image.ImageID, image.OSSURL)
		}
	}
}

func (s *TrackSubmissionService) decorateImages(ctx context.Context, sub *models.TrackSubmission) {
	if sub == nil {
		return
	}
	if sub.Images == nil {
		sub.Images = []*models.TrackSubmissionImage{}
	}
	if s.imageCache == nil {
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, image := range sub.Images {
		if image != nil {
			image.URL = s.imageCache.EnsureCached(cacheCtx, sub.UserID, sub.SubmissionID+"_"+image.ImageID, image.OSSURL)
		}
	}
}

func (s *TrackSubmissionService) ListByTrackIDs(ctx context.Context, trackIDs []string) (map[string]*models.TrackSubmission, error) {
	return s.repo.ListByTrackIDs(ctx, trackIDs)
}

func (s *TrackSubmissionService) DecorateSummaries(ctx context.Context, summaries []*models.TrackSummary) error {
	ids := make([]string, 0, len(summaries))
	for _, item := range summaries {
		if item != nil {
			ids = append(ids, item.ID)
		}
	}
	byTrack, err := s.repo.ListByTrackIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, item := range summaries {
		if item == nil {
			continue
		}
		sub := byTrack[item.ID]
		if sub == nil || sub.Status != models.TrackSubmissionStatusApproved {
			continue
		}
		item.IsFeatured, item.Title, item.FeaturedDescription = true, sub.Title, sub.Description
		item.FeaturedCoverURL = item.TrackScreenshotURL
		if len(sub.Images) > 0 && sub.Images[0] != nil && s.imageCache != nil {
			cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			item.FeaturedCoverURL = s.imageCache.EnsureCached(cacheCtx, sub.UserID, sub.SubmissionID+"_"+sub.Images[0].ImageID, sub.Images[0].OSSURL)
			cancel()
			if item.FeaturedCoverURL == "" {
				item.FeaturedCoverURL = item.TrackScreenshotURL
			}
		}
	}
	return nil
}

func (s *TrackSubmissionService) DecorateMySummaries(ctx context.Context, summaries []*models.MyTrackSummary) error {
	ids := make([]string, 0, len(summaries))
	for _, item := range summaries {
		if item != nil {
			ids = append(ids, item.ID)
		}
	}
	byTrack, err := s.repo.ListByTrackIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, item := range summaries {
		if item == nil {
			continue
		}
		if sub := byTrack[item.ID]; sub != nil {
			item.Submission = &models.TrackSubmissionSummary{Status: sub.Status, Revision: sub.Revision, ReviewReason: sub.ReviewReason}
		}
	}
	return nil
}

func (s *TrackSubmissionService) PublicForTrack(ctx context.Context, trackID string) (*models.TrackSubmission, error) {
	sub, err := s.repo.FindByTrackID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if sub.Status != models.TrackSubmissionStatusApproved {
		return nil, repository.ErrNotFound
	}
	sub.ReviewedBy, sub.ReviewReason = "", ""
	s.decorateImages(ctx, sub)
	return sub, nil
}

func (s *TrackSubmissionService) ApprovedTrackIDs(ctx context.Context, trackIDs []string) (map[string]bool, error) {
	result := make(map[string]bool)
	items, err := s.repo.ListByTrackIDs(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	for trackID, sub := range items {
		if sub != nil && sub.Status == models.TrackSubmissionStatusApproved {
			result[trackID] = true
		}
	}
	return result, nil
}
