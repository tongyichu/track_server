package service

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type fakeAnalyticsUploader struct {
	uploads map[string]string
	err     error
}

func (f *fakeAnalyticsUploader) UploadLocalFileToOSS(objectKey, localPath string) error {
	if f.err != nil {
		return f.err
	}
	if f.uploads == nil {
		f.uploads = make(map[string]string)
	}
	f.uploads[objectKey] = localPath
	return nil
}

func TestAnalyticsServiceIngestWritesSanitizedJSONL(t *testing.T) {
	now := time.Date(2026, 6, 12, 15, 4, 5, 0, time.UTC)
	dir := t.TempDir()
	svc, err := NewAnalyticsService(AnalyticsConfig{
		Enabled:      true,
		LocalDir:     dir,
		OSSPrefix:    "analytics/ods",
		InstanceID:   "test-instance",
		MaxBatchSize: 10,
		Now:          func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewAnalyticsService failed: %v", err)
	}
	result, err := svc.Ingest(context.Background(), AnalyticsEventBatch{
		Events: []map[string]any{
			{
				"event_id":    "evt-1",
				"event_name":  "track_create_success",
				"phone":       "13800000000",
				"latitude":    39.9,
				"oss_url":     "https://bucket.oss-cn.aliyuncs.com/a.jpg?OSSAccessKeyId=secret",
				"app_version": "",
			},
		},
	}, AnalyticsIngestMeta{
		UserID:     1001,
		Platform:   "ios",
		AppVersion: "1.0.0",
		DeviceID:   "anon-1",
		ClientLang: "zh-CN",
	})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if result.Accepted != 1 {
		t.Fatalf("accepted=%d, want 1", result.Accepted)
	}
	path := filepath.Join(dir, "2026-06-12", "15", "events-test-instance-000001.jsonl.writing")
	rows, err := readAnalyticsJSONLLines(path)
	if err != nil {
		t.Fatalf("read jsonl failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1", len(rows))
	}
	row := rows[0]
	if row["event_id"] != "evt-1" || row["event_name"] != "track_create_success" {
		t.Fatalf("unexpected event row: %#v", row)
	}
	if _, ok := row["phone"]; ok {
		t.Fatalf("phone should be removed: %#v", row)
	}
	if _, ok := row["latitude"]; ok {
		t.Fatalf("latitude should be removed: %#v", row)
	}
	if row["server_user_id"] != "1001" || row["platform"] != "ios" || row["app_version"] != "1.0.0" {
		t.Fatalf("request metadata not applied: %#v", row)
	}
}

func TestAnalyticsServiceSyncClosedFilesUploadsAndRemovesLocalFile(t *testing.T) {
	now := time.Date(2026, 6, 12, 15, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	uploader := &fakeAnalyticsUploader{}
	repo := repository.NewInMemoryAnalyticsRepository()
	svc, err := NewAnalyticsService(AnalyticsConfig{
		Enabled:    true,
		LocalDir:   dir,
		OSSPrefix:  "analytics/ods",
		InstanceID: "inst-1",
		Now:        func() time.Time { return now },
		Uploader:   uploader,
		Repository: repo,
	})
	if err != nil {
		t.Fatalf("NewAnalyticsService failed: %v", err)
	}
	if _, err := svc.Ingest(context.Background(), AnalyticsEventBatch{Events: []map[string]any{{"event_id": "evt-1", "event_name": "app_launch"}}}, AnalyticsIngestMeta{}); err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	result, err := svc.SyncClosedFiles(context.Background())
	if err != nil {
		t.Fatalf("SyncClosedFiles failed: %v", err)
	}
	if result.Uploaded != 1 || result.Failed != 0 {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	if result.TotalBytes <= 0 {
		t.Fatalf("total bytes=%d, want >0", result.TotalBytes)
	}
	if result.Summary == nil || result.Summary.ID == 0 {
		t.Fatalf("result summary missing: %#v", result.Summary)
	}
	key := "analytics/ods/event_date=2026-06-12/hour=15/events-inst-1-000001.jsonl"
	if uploader.uploads[key] == "" {
		t.Fatalf("missing upload for key %s; uploads=%#v", key, uploader.uploads)
	}
	if _, err := os.Stat(uploader.uploads[key]); !os.IsNotExist(err) {
		t.Fatalf("uploaded source should have been removed, stat err=%v", err)
	}
	summaries := repo.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries=%d, want 1", len(summaries))
	}
	summary := summaries[0]
	if summary.Status != models.AnalyticsSyncStatusSuccess || summary.UploadedFiles != 1 || summary.TotalBytes != result.TotalBytes {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.OSSPrefix != "analytics/ods" || summary.FilesJSON == "" || summary.FilesJSON == "[]" {
		t.Fatalf("summary missing oss/files detail: %#v", summary)
	}
}

func readAnalyticsJSONLLines(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, scanner.Err()
}
