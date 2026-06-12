package jobs

import (
	"context"
	"testing"

	"github.com/tongyichu/track_server/internal/service"
)

func TestAnalyticsSyncDefaultsToThreeAM(t *testing.T) {
	job := NewAnalyticsSync(nil, "")
	if job.Spec() != "0 3 * * *" {
		t.Fatalf("spec=%q, want 0 3 * * *", job.Spec())
	}
	if job.Name() != "analytics_sync" {
		t.Fatalf("name=%q", job.Name())
	}
}

func TestAnalyticsSyncRun(t *testing.T) {
	dir := t.TempDir()
	uploader := &fakeJobAnalyticsUploader{}
	svc, err := service.NewAnalyticsService(service.AnalyticsConfig{
		Enabled:    true,
		LocalDir:   dir,
		OSSPrefix:  "analytics/ods",
		InstanceID: "job-inst",
		Uploader:   uploader,
	})
	if err != nil {
		t.Fatalf("NewAnalyticsService failed: %v", err)
	}
	if _, err := svc.Ingest(context.Background(), service.AnalyticsEventBatch{Events: []map[string]any{{"event_id": "evt-1", "event_name": "app_launch"}}}, service.AnalyticsIngestMeta{}); err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	job := NewAnalyticsSync(svc, "@every 1h")
	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if uploader.count != 1 {
		t.Fatalf("upload count=%d, want 1", uploader.count)
	}
}

type fakeJobAnalyticsUploader struct {
	count int
}

func (f *fakeJobAnalyticsUploader) UploadLocalFileToOSS(objectKey, localPath string) error {
	f.count++
	return nil
}
