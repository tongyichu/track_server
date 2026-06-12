package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	defaultAnalyticsMaxBatchSize = 50
	defaultAnalyticsMaxBodyBytes = int64(256 * 1024)
	defaultAnalyticsMaxFileBytes = int64(64 * 1024 * 1024)
	defaultAnalyticsRotateAfter  = 5 * time.Minute
	defaultAnalyticsSchema       = "1"
)

var (
	ErrAnalyticsDisabled  = errors.New("analytics disabled")
	ErrAnalyticsTooLarge  = errors.New("analytics payload too large")
	ErrAnalyticsBadEvents = errors.New("analytics events invalid")
)

// AnalyticsOSSUploader abstracts OSS upload for local analytics files.
type AnalyticsOSSUploader interface {
	UploadLocalFileToOSS(objectKey, localPath string) error
}

// AnalyticsConfig contains runtime knobs for local analytics ingestion.
type AnalyticsConfig struct {
	Enabled      bool
	LocalDir     string
	OSSPrefix    string
	InstanceID   string
	MaxBatchSize int
	MaxBodyBytes int64
	MaxFileBytes int64
	RotateAfter  time.Duration
	Now          func() time.Time
	Uploader     AnalyticsOSSUploader
	Repository   repository.AnalyticsRepository
}

// AnalyticsEventBatch is accepted by POST /api/v1/analytics/events.
type AnalyticsEventBatch struct {
	Events []map[string]any `json:"events"`
}

// AnalyticsIngestMeta contains trustworthy request-side metadata.
type AnalyticsIngestMeta struct {
	UserID        int64
	Platform      string
	AppVersion    string
	DeviceID      string
	ClientLang    string
	RemoteIP      string
	RequestID     string
	Authorization bool
}

// AnalyticsIngestResult is returned to clients after local persistence.
type AnalyticsIngestResult struct {
	Accepted int    `json:"accepted"`
	Status   string `json:"status"`
}

// AnalyticsSyncResult describes one OSS synchronization run.
type AnalyticsSyncResult struct {
	Scanned    int                               `json:"scanned"`
	Uploaded   int                               `json:"uploaded"`
	Failed     int                               `json:"failed"`
	TotalBytes int64                             `json:"total_bytes"`
	Files      []models.AnalyticsSyncFileSummary `json:"files,omitempty"`
	Summary    *models.AnalyticsSyncSummary      `json:"summary,omitempty"`
}

// AnalyticsService receives client analytics events, writes local JSONL files,
// and uploads closed files to OSS through a scheduled job.
type AnalyticsService struct {
	enabled      bool
	localDir     string
	ossPrefix    string
	instanceID   string
	maxBatchSize int
	maxBodyBytes int64
	maxFileBytes int64
	rotateAfter  time.Duration
	now          func() time.Time
	uploader     AnalyticsOSSUploader
	repo         repository.AnalyticsRepository

	mu           sync.Mutex
	currentFile  *os.File
	currentPath  string
	currentSize  int64
	currentStart time.Time
	seq          int64
}

// NewAnalyticsService constructs an AnalyticsService.
func NewAnalyticsService(cfg AnalyticsConfig) (*AnalyticsService, error) {
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = defaultAnalyticsMaxBatchSize
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultAnalyticsMaxBodyBytes
	}
	if cfg.MaxFileBytes <= 0 {
		cfg.MaxFileBytes = defaultAnalyticsMaxFileBytes
	}
	if cfg.RotateAfter <= 0 {
		cfg.RotateAfter = defaultAnalyticsRotateAfter
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if strings.TrimSpace(cfg.LocalDir) == "" {
		return nil, invalidArg("analytics local dir is required")
	}
	instanceID := sanitizeAnalyticsPathPart(cfg.InstanceID)
	if instanceID == "" {
		instanceID = defaultAnalyticsInstanceID()
	}
	ossPrefix := strings.Trim(strings.TrimSpace(cfg.OSSPrefix), "/")
	if ossPrefix == "" {
		ossPrefix = "analytics/ods"
	}
	if err := os.MkdirAll(cfg.LocalDir, 0o755); err != nil {
		return nil, err
	}
	return &AnalyticsService{
		enabled:      cfg.Enabled,
		localDir:     cfg.LocalDir,
		ossPrefix:    ossPrefix,
		instanceID:   instanceID,
		maxBatchSize: cfg.MaxBatchSize,
		maxBodyBytes: cfg.MaxBodyBytes,
		maxFileBytes: cfg.MaxFileBytes,
		rotateAfter:  cfg.RotateAfter,
		now:          cfg.Now,
		uploader:     cfg.Uploader,
		repo:         cfg.Repository,
	}, nil
}

func (s *AnalyticsService) MaxBodyBytes() int64 {
	if s == nil || s.maxBodyBytes <= 0 {
		return defaultAnalyticsMaxBodyBytes
	}
	return s.maxBodyBytes
}

func (s *AnalyticsService) Ingest(ctx context.Context, batch AnalyticsEventBatch, meta AnalyticsIngestMeta) (AnalyticsIngestResult, error) {
	if s == nil || !s.enabled {
		return AnalyticsIngestResult{}, ErrAnalyticsDisabled
	}
	if len(batch.Events) == 0 || len(batch.Events) > s.maxBatchSize {
		return AnalyticsIngestResult{}, fmt.Errorf("%w: events length must be 1..%d", ErrAnalyticsBadEvents, s.maxBatchSize)
	}
	now := s.now().UTC()
	lines := make([][]byte, 0, len(batch.Events))
	for i, event := range batch.Events {
		line, err := s.normalizeEvent(event, meta, now)
		if err != nil {
			return AnalyticsIngestResult{}, fmt.Errorf("%w: event[%d]: %v", ErrAnalyticsBadEvents, i, err)
		}
		lines = append(lines, line)
	}
	if err := ctx.Err(); err != nil {
		return AnalyticsIngestResult{}, err
	}
	if err := s.appendLines(lines, now); err != nil {
		return AnalyticsIngestResult{}, err
	}
	return AnalyticsIngestResult{Accepted: len(lines), Status: "ok"}, nil
}

func (s *AnalyticsService) normalizeEvent(event map[string]any, meta AnalyticsIngestMeta, now time.Time) ([]byte, error) {
	if event == nil {
		return nil, errors.New("event must be object")
	}
	eventID := strings.TrimSpace(asString(event["event_id"]))
	if eventID == "" {
		return nil, errors.New("event_id is required")
	}
	eventName := strings.TrimSpace(asString(event["event_name"]))
	if eventName == "" {
		return nil, errors.New("event_name is required")
	}
	if len(eventID) > 128 {
		return nil, errors.New("event_id too long")
	}
	if len(eventName) > 128 {
		return nil, errors.New("event_name too long")
	}
	clean, _ := sanitizeAnalyticsValue(event).(map[string]any)
	if clean == nil {
		clean = make(map[string]any)
	}
	clean["event_id"] = eventID
	clean["event_name"] = eventName
	clean["server_time"] = now.Format(time.RFC3339Nano)
	clean["schema_version"] = defaultAnalyticsSchema
	if meta.UserID > 0 {
		clean["server_user_id"] = strconv.FormatInt(meta.UserID, 10)
		if strings.TrimSpace(asString(clean["user_id"])) == "" {
			clean["user_id"] = strconv.FormatInt(meta.UserID, 10)
		}
	}
	if meta.Platform != "" && strings.TrimSpace(asString(clean["platform"])) == "" {
		clean["platform"] = meta.Platform
	}
	if meta.AppVersion != "" && strings.TrimSpace(asString(clean["app_version"])) == "" {
		clean["app_version"] = meta.AppVersion
	}
	if meta.DeviceID != "" && strings.TrimSpace(asString(clean["anonymous_id"])) == "" {
		clean["anonymous_id"] = meta.DeviceID
	}
	if meta.ClientLang != "" && strings.TrimSpace(asString(clean["locale"])) == "" {
		clean["locale"] = meta.ClientLang
	}
	if meta.RemoteIP != "" {
		clean["ip_region"] = meta.RemoteIP
	}
	if meta.Authorization {
		clean["auth_state"] = "authorized"
	}
	return json.Marshal(clean)
}

func (s *AnalyticsService) appendLines(lines [][]byte, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range lines {
		if err := s.ensureCurrentFile(now, int64(len(line)+1)); err != nil {
			return err
		}
		n, err := s.currentFile.Write(append(line, '\n'))
		if err != nil {
			return err
		}
		s.currentSize += int64(n)
	}
	if s.currentFile != nil {
		return s.currentFile.Sync()
	}
	return nil
}

func (s *AnalyticsService) ensureCurrentFile(now time.Time, nextBytes int64) error {
	if s.currentFile != nil {
		tooLarge := s.currentSize > 0 && s.currentSize+nextBytes > s.maxFileBytes
		tooOld := s.currentStart.IsZero() || now.Sub(s.currentStart) >= s.rotateAfter
		if !tooLarge && !tooOld {
			return nil
		}
		if err := s.closeCurrentLocked(); err != nil {
			return err
		}
	}
	dir := filepath.Join(s.localDir, now.Format("2006-01-02"), now.Format("15"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.seq++
	name := fmt.Sprintf("events-%s-%06d.jsonl.writing", s.instanceID, s.seq)
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	s.currentFile = file
	s.currentPath = path
	s.currentSize = 0
	s.currentStart = now
	return nil
}

func (s *AnalyticsService) closeCurrentLocked() error {
	if s.currentFile == nil {
		return nil
	}
	path := s.currentPath
	if err := s.currentFile.Sync(); err != nil {
		_ = s.currentFile.Close()
		return err
	}
	if err := s.currentFile.Close(); err != nil {
		return err
	}
	closedPath := strings.TrimSuffix(path, ".writing")
	if closedPath == path {
		closedPath = path + ".jsonl"
	}
	if err := os.Rename(path, closedPath); err != nil {
		return err
	}
	s.currentFile = nil
	s.currentPath = ""
	s.currentSize = 0
	s.currentStart = time.Time{}
	return nil
}

// SyncClosedFiles closes the active file and uploads all closed JSONL files to OSS.
func (s *AnalyticsService) SyncClosedFiles(ctx context.Context) (result AnalyticsSyncResult, retErr error) {
	startedAt := time.Now().UTC()
	realStart := time.Now()
	var syncErr error
	defer func() {
		endedAt := time.Now().UTC()
		result.Summary = s.recordSyncSummary(startedAt, endedAt, time.Since(realStart), result, syncErr)
	}()
	if s == nil || !s.enabled {
		syncErr = ErrAnalyticsDisabled
		return result, syncErr
	}
	if s.uploader == nil {
		syncErr = errors.New("analytics oss uploader not configured")
		return result, syncErr
	}
	s.mu.Lock()
	err := s.closeCurrentLocked()
	s.mu.Unlock()
	if err != nil {
		syncErr = err
		return result, syncErr
	}

	walkErr := filepath.WalkDir(s.localDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		fileSummary := models.AnalyticsSyncFileSummary{
			LocalPath: path,
			Status:    models.AnalyticsSyncStatusFailed,
		}
		if info, err := d.Info(); err == nil {
			fileSummary.SizeBytes = info.Size()
		}
		result.Scanned++
		objectKey, err := s.objectKeyForLocalFile(path)
		if err != nil {
			result.Failed++
			fileSummary.Error = err.Error()
			result.Files = append(result.Files, fileSummary)
			log.Printf("[Analytics] object_key_error path=%s err=%v", path, err)
			return nil
		}
		fileSummary.OSSKey = objectKey
		if err := s.uploader.UploadLocalFileToOSS(objectKey, path); err != nil {
			result.Failed++
			fileSummary.Error = err.Error()
			result.Files = append(result.Files, fileSummary)
			log.Printf("[Analytics] upload_failed path=%s object_key=%s err=%v", path, objectKey, err)
			return nil
		}
		if err := s.removeUploadedLocalFile(path); err != nil {
			result.Failed++
			fileSummary.Error = err.Error()
			result.Files = append(result.Files, fileSummary)
			log.Printf("[Analytics] cleanup_uploaded_failed path=%s err=%v", path, err)
			return nil
		}
		result.Uploaded++
		result.TotalBytes += fileSummary.SizeBytes
		fileSummary.Status = models.AnalyticsSyncStatusSuccess
		result.Files = append(result.Files, fileSummary)
		return nil
	})
	if walkErr != nil {
		syncErr = walkErr
		return result, syncErr
	}
	if result.Failed > 0 {
		syncErr = fmt.Errorf("analytics sync failed: uploaded=%d failed=%d", result.Uploaded, result.Failed)
		return result, syncErr
	}
	return result, nil
}

func (s *AnalyticsService) recordSyncSummary(startedAt, endedAt time.Time, duration time.Duration, result AnalyticsSyncResult, syncErr error) *models.AnalyticsSyncSummary {
	if s == nil || s.repo == nil {
		return nil
	}
	status := models.AnalyticsSyncStatusSuccess
	if syncErr != nil {
		status = models.AnalyticsSyncStatusFailed
		if result.Uploaded > 0 {
			status = models.AnalyticsSyncStatusPartial
		}
	}
	filesJSON := "[]"
	if b, err := json.Marshal(result.Files); err == nil {
		filesJSON = string(b)
	}
	errorMessage := ""
	if syncErr != nil {
		errorMessage = syncErr.Error()
	}
	summary := &models.AnalyticsSyncSummary{
		JobName:       "analytics_sync",
		Status:        status,
		StartedAt:     startedAt,
		EndedAt:       endedAt,
		DurationMS:    duration.Milliseconds(),
		ScannedFiles:  result.Scanned,
		UploadedFiles: result.Uploaded,
		FailedFiles:   result.Failed,
		TotalBytes:    result.TotalBytes,
		OSSPrefix:     s.ossPrefix,
		FilesJSON:     filesJSON,
		ErrorMessage:  errorMessage,
		CreatedAt:     endedAt,
	}
	recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repo.CreateSyncSummary(recordCtx, summary); err != nil {
		log.Printf("[Analytics] sync_summary_record_failed status=%s scanned=%d uploaded=%d failed=%d err=%v", status, result.Scanned, result.Uploaded, result.Failed, err)
		return nil
	}
	return summary
}

func (s *AnalyticsService) objectKeyForLocalFile(path string) (string, error) {
	rel, err := filepath.Rel(s.localDir, path)
	if err != nil {
		return "", err
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 3 {
		return "", fmt.Errorf("unexpected analytics path %q", rel)
	}
	date := sanitizeAnalyticsPathPart(parts[0])
	hour := sanitizeAnalyticsPathPart(parts[1])
	file := sanitizeAnalyticsPathPart(parts[len(parts)-1])
	if date == "" || hour == "" || file == "" {
		return "", fmt.Errorf("invalid analytics path %q", rel)
	}
	return fmt.Sprintf("%s/event_date=%s/hour=%s/%s", s.ossPrefix, date, hour, file), nil
}

func (s *AnalyticsService) removeUploadedLocalFile(path string) error {
	rel, err := filepath.Rel(s.localDir, path)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	dir := filepath.Join(s.localDir, filepath.Dir(rel))
	for dir != s.localDir && strings.HasPrefix(dir, s.localDir) {
		if err := os.Remove(dir); err != nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

func sanitizeAnalyticsValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			key := strings.TrimSpace(k)
			if key == "" || isSensitiveAnalyticsKey(key) {
				continue
			}
			out[key] = sanitizeAnalyticsValue(v)
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, sanitizeAnalyticsValue(item))
		}
		return out
	case string:
		return sanitizeAnalyticsString(x)
	default:
		return v
	}
}

func isSensitiveAnalyticsKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "phone", "mobile", "mobile_phone", "sms_code", "captcha", "authorization", "jwt", "token", "access_token", "refresh_token",
		"security_token", "access_key_secret", "password", "oss_url", "image_url", "raw_track_url", "track_points", "points",
		"coordinates", "latitude", "longitude", "lat", "lng":
		return true
	}
	return strings.Contains(k, "token") || strings.Contains(k, "password") || strings.Contains(k, "secret")
}

func sanitizeAnalyticsString(s string) string {
	if strings.Contains(s, "OSSAccessKeyId=") || strings.Contains(strings.ToLower(s), "x-oss-signature") {
		if idx := strings.Index(s, "?"); idx >= 0 {
			return s[:idx]
		}
	}
	return s
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	default:
		return ""
	}
}

func sanitizeAnalyticsPathPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func defaultAnalyticsInstanceID() string {
	host, _ := os.Hostname()
	host = sanitizeAnalyticsPathPart(host)
	if host != "" {
		return host
	}
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return "instance"
}
