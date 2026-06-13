package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	MaxFeedbackImages       = 3
	MaxFeedbackImageSize    = 5 * 1024 * 1024
	MaxFeedbackImagesSize   = MaxFeedbackImages * MaxFeedbackImageSize
	MaxOpenFeedbackPerUser  = 5
	defaultFeedbackPageSize = 20
	maxFeedbackPageSize     = 50
)

var (
	ErrFeedbackImageTooMany      = errors.New("feedback images cannot exceed 3")
	ErrFeedbackImageTooLarge     = errors.New("feedback image too large")
	ErrFeedbackImageType         = errors.New("unsupported feedback image type")
	ErrFeedbackContentRequired   = errors.New("feedback content is required")
	ErrFeedbackOpenLimitExceeded = errors.New("too many unprocessed feedbacks")
	ErrFeedbackReplyRequired     = errors.New("reply is required when feedback is resolved")
	openFeedbackStatuses         = []models.FeedbackStatus{models.FeedbackStatusPending, models.FeedbackStatusProcessing}
)

type FeedbackService struct {
	repo     repository.FeedbackRepository
	imageDir string
}

type SubmitFeedbackInput struct {
	UserID        int64
	Content       string
	Images        []*multipart.FileHeader
	Contact       string
	AppVersion    string
	Platform      string
	DeviceModel   string
	SystemVersion string
}

type UpdateFeedbackStatusInput struct {
	FeedbackID string
	Status     models.FeedbackStatus
	Reply      string
}

type FeedbackImageFile struct {
	Path     string
	MimeType string
}

func NewFeedbackService(repo repository.FeedbackRepository, imageDir string) *FeedbackService {
	return &FeedbackService{repo: repo, imageDir: strings.TrimSpace(imageDir)}
}

func (s *FeedbackService) Submit(ctx context.Context, in SubmitFeedbackInput) (*models.Feedback, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("feedback service not configured")
	}
	if s.imageDir == "" {
		return nil, errors.New("feedback image dir not configured")
	}
	content := strings.TrimSpace(in.Content)
	if content == "" || utf8.RuneCountInString(content) > 1000 {
		return nil, invalidArg("content is required and must be <= 1000 characters")
	}
	if in.UserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	if len(in.Images) > MaxFeedbackImages {
		return nil, ErrFeedbackImageTooMany
	}
	var totalSize int64
	for _, fh := range in.Images {
		if fh == nil {
			continue
		}
		if fh.Size > MaxFeedbackImageSize {
			return nil, ErrFeedbackImageTooLarge
		}
		totalSize += fh.Size
	}
	if totalSize > MaxFeedbackImagesSize {
		return nil, ErrFeedbackImageTooLarge
	}
	openCount, err := s.repo.CountByUserAndStatuses(ctx, in.UserID, openFeedbackStatuses)
	if err != nil {
		return nil, err
	}
	if openCount >= MaxOpenFeedbackPerUser {
		return nil, ErrFeedbackOpenLimitExceeded
	}

	now := time.Now()
	feedbackID, err := newFeedbackID(now)
	if err != nil {
		return nil, err
	}
	feedback := &models.Feedback{
		FeedbackID:    feedbackID,
		UserID:        in.UserID,
		Content:       content,
		Contact:       trimMax(in.Contact, 128),
		AppVersion:    trimMax(in.AppVersion, 32),
		Platform:      trimMax(in.Platform, 32),
		DeviceModel:   trimMax(in.DeviceModel, 128),
		SystemVersion: trimMax(in.SystemVersion, 64),
		Status:        models.FeedbackStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	images, err := s.saveImages(in.UserID, feedbackID, now, in.Images)
	if err != nil {
		return nil, err
	}
	feedback.Images = images
	if err := s.repo.Create(ctx, feedback); err != nil {
		return nil, err
	}
	s.fillImageURLs(feedback)
	return feedback, nil
}

func (s *FeedbackService) ListMine(ctx context.Context, userID int64, cursor *models.FeedbackListCursor, limit int) (*models.FeedbackPage, error) {
	return s.list(ctx, models.FeedbackListFilter{UserID: userID, Cursor: cursor, Limit: normalizeFeedbackLimit(limit) + 1}, normalizeFeedbackLimit(limit))
}

func (s *FeedbackService) ListOps(ctx context.Context, status models.FeedbackStatus, cursor *models.FeedbackListCursor, limit int) (*models.FeedbackPage, error) {
	return s.ListOpsFiltered(ctx, models.FeedbackListFilter{Status: status, Cursor: cursor}, limit)
}

func (s *FeedbackService) ListOpsFiltered(ctx context.Context, filter models.FeedbackListFilter, limit int) (*models.FeedbackPage, error) {
	filter.AppVersion = strings.TrimSpace(filter.AppVersion)
	if filter.Status != "" && !isValidFeedbackStatus(filter.Status) {
		return nil, invalidArg("invalid status")
	}
	filter.Limit = normalizeFeedbackLimit(limit) + 1
	return s.list(ctx, filter, normalizeFeedbackLimit(limit))
}

func (s *FeedbackService) list(ctx context.Context, filter models.FeedbackListFilter, limit int) (*models.FeedbackPage, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("feedback service not configured")
	}
	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	for _, item := range items {
		s.fillImageURLs(item)
	}
	var next string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		next = EncodeFeedbackCursor(&models.FeedbackListCursor{CreatedAt: last.CreatedAt, FeedbackID: last.FeedbackID})
	}
	return &models.FeedbackPage{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

func (s *FeedbackService) GetMine(ctx context.Context, userID int64, feedbackID string) (*models.Feedback, error) {
	item, err := s.get(ctx, feedbackID)
	if err != nil {
		return nil, err
	}
	if item.UserID != userID {
		return nil, repository.ErrForbidden
	}
	return item, nil
}

func (s *FeedbackService) GetOps(ctx context.Context, feedbackID string) (*models.Feedback, error) {
	return s.get(ctx, feedbackID)
}

func (s *FeedbackService) get(ctx context.Context, feedbackID string) (*models.Feedback, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("feedback service not configured")
	}
	feedbackID = strings.TrimSpace(feedbackID)
	if feedbackID == "" {
		return nil, invalidArg("feedback_id is required")
	}
	item, err := s.repo.FindByFeedbackID(ctx, feedbackID)
	if err != nil {
		return nil, err
	}
	s.fillImageURLs(item)
	return item, nil
}

func (s *FeedbackService) UpdateStatus(ctx context.Context, in UpdateFeedbackStatusInput) error {
	if s == nil || s.repo == nil {
		return errors.New("feedback service not configured")
	}
	if strings.TrimSpace(in.FeedbackID) == "" {
		return invalidArg("feedback_id is required")
	}
	if !isValidFeedbackStatus(in.Status) {
		return invalidArg("invalid status")
	}
	reply := trimMax(in.Reply, 2000)
	if in.Status == models.FeedbackStatusResolved && reply == "" {
		return ErrFeedbackReplyRequired
	}
	return s.repo.UpdateStatus(ctx, strings.TrimSpace(in.FeedbackID), in.Status, reply)
}

func (s *FeedbackService) GetImageFile(ctx context.Context, userID int64, feedbackID, imageID string, allowAny bool) (*FeedbackImageFile, error) {
	item, err := s.get(ctx, feedbackID)
	if err != nil {
		return nil, err
	}
	if !allowAny && item.UserID != userID {
		return nil, repository.ErrForbidden
	}
	for _, img := range item.Images {
		if img.ImageID != imageID {
			continue
		}
		path, err := s.resolveFeedbackImagePath(item, img)
		if err != nil {
			return nil, err
		}
		return &FeedbackImageFile{Path: path, MimeType: img.MimeType}, nil
	}
	return nil, repository.ErrNotFound
}

func (s *FeedbackService) saveImages(userID int64, feedbackID string, now time.Time, headers []*multipart.FileHeader) ([]models.FeedbackImage, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	day := now.Format("20060102")
	relDir := filepath.Join(fmt.Sprintf("%d", userID), day)
	dstDir := filepath.Join(s.imageDir, relDir)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, err
	}
	images := make([]models.FeedbackImage, 0, len(headers))
	for idx, fh := range headers {
		if fh == nil {
			continue
		}
		img, err := s.saveOneImage(fh, dstDir, relDir, feedbackID, idx+1)
		if err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, nil
}

func (s *FeedbackService) saveOneImage(fh *multipart.FileHeader, dstDir, relDir, feedbackID string, index int) (models.FeedbackImage, error) {
	src, err := fh.Open()
	if err != nil {
		return models.FeedbackImage{}, err
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, MaxFeedbackImageSize+1))
	if err != nil {
		return models.FeedbackImage{}, err
	}
	if int64(len(data)) > MaxFeedbackImageSize {
		return models.FeedbackImage{}, ErrFeedbackImageTooLarge
	}
	mimeType, ext, ok := detectFeedbackImage(data)
	if !ok {
		return models.FeedbackImage{}, ErrFeedbackImageType
	}
	imageID := fmt.Sprintf("%d", index)
	fileName := fmt.Sprintf("%s_%d%s", feedbackID, index, ext)
	dstPath := filepath.Join(dstDir, fileName)
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		return models.FeedbackImage{}, err
	}
	return models.FeedbackImage{
		ImageID:     imageID,
		StoragePath: filepath.ToSlash(filepath.Join(relDir, fileName)),
		MimeType:    mimeType,
		Size:        int64(len(data)),
	}, nil
}

func (s *FeedbackService) resolveImagePath(storagePath string) (string, error) {
	if s == nil || s.imageDir == "" {
		return "", errors.New("feedback image dir not configured")
	}
	clean := filepath.Clean(strings.TrimSpace(storagePath))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", repository.ErrNotFound
	}
	path := filepath.Join(s.imageDir, clean)
	base, err := filepath.Abs(s.imageDir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if abs != base && !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		return "", repository.ErrNotFound
	}
	return abs, nil
}

func (s *FeedbackService) resolveFeedbackImagePath(item *models.Feedback, img models.FeedbackImage) (string, error) {
	storagePath := strings.TrimSpace(img.StoragePath)
	if storagePath == "" {
		storagePath = legacyFeedbackImageStoragePath(item, img)
	}
	return s.resolveImagePath(storagePath)
}

func legacyFeedbackImageStoragePath(item *models.Feedback, img models.FeedbackImage) string {
	if item == nil || item.UserID <= 0 || item.FeedbackID == "" || item.CreatedAt.IsZero() {
		return ""
	}
	if !isSafeFeedbackPathPart(item.FeedbackID) || !isSafeFeedbackPathPart(img.ImageID) {
		return ""
	}
	ext := feedbackImageExt(img.MimeType)
	if ext == "" {
		return ""
	}
	day := item.CreatedAt.Format("20060102")
	fileName := fmt.Sprintf("%s_%s%s", item.FeedbackID, img.ImageID, ext)
	return filepath.ToSlash(filepath.Join(fmt.Sprintf("%d", item.UserID), day, fileName))
}

func feedbackImageExt(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func isSafeFeedbackPathPart(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			continue
		default:
			return false
		}
	}
	return true
}

func (s *FeedbackService) fillImageURLs(item *models.Feedback) {
	if item == nil {
		return
	}
	for i := range item.Images {
		item.Images[i].URL = fmt.Sprintf("/api/v1/feedback/%s/images/%s", item.FeedbackID, item.Images[i].ImageID)
	}
}

func detectFeedbackImage(data []byte) (mimeType, ext string, ok bool) {
	if len(data) < 12 {
		return "", "", false
	}
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp", ".webp", true
	}
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return "image/jpeg", ".jpg", true
	case "image/png":
		return "image/png", ".png", true
	case "image/webp":
		return "image/webp", ".webp", true
	default:
		return "", "", false
	}
}

func newFeedbackID(now time.Time) (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "FB" + now.Format("20060102150405") + strings.ToUpper(hex.EncodeToString(b[:])), nil
}

func isValidFeedbackStatus(status models.FeedbackStatus) bool {
	switch status {
	case models.FeedbackStatusPending, models.FeedbackStatusProcessing, models.FeedbackStatusResolved, models.FeedbackStatusIgnored:
		return true
	default:
		return false
	}
}

func normalizeFeedbackLimit(limit int) int {
	if limit <= 0 {
		return defaultFeedbackPageSize
	}
	if limit > maxFeedbackPageSize {
		return maxFeedbackPageSize
	}
	return limit
}

func ParseFeedbackCursor(raw string) (*models.FeedbackListCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, invalidArg("invalid cursor")
	}
	var cursor models.FeedbackListCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, invalidArg("invalid cursor")
	}
	if cursor.CreatedAt.IsZero() || cursor.FeedbackID == "" {
		return nil, invalidArg("invalid cursor")
	}
	return &cursor, nil
}

func EncodeFeedbackCursor(cursor *models.FeedbackListCursor) string {
	if cursor == nil {
		return ""
	}
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func trimMax(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
