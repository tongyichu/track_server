package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/tongyichu/track_server/internal/service"
)

const defaultAnalyticsSyncSpec = "0 3 * * *"

// AnalyticsSync uploads closed local analytics JSONL files to OSS ODS.
type AnalyticsSync struct {
	analyticsSvc *service.AnalyticsService
	spec         string
}

func NewAnalyticsSync(analyticsSvc *service.AnalyticsService, spec string) *AnalyticsSync {
	if strings.TrimSpace(spec) == "" {
		spec = defaultAnalyticsSyncSpec
	}
	return &AnalyticsSync{analyticsSvc: analyticsSvc, spec: spec}
}

func (j *AnalyticsSync) Name() string { return "analytics_sync" }

func (j *AnalyticsSync) Spec() string { return j.spec }

func (j *AnalyticsSync) Run(ctx context.Context) error {
	if j.analyticsSvc == nil {
		return fmt.Errorf("analytics_sync: service is nil")
	}
	result, err := j.analyticsSvc.SyncClosedFiles(ctx)
	log.Printf("[scheduler] analytics_sync: scanned=%d uploaded=%d failed=%d", result.Scanned, result.Uploaded, result.Failed)
	return err
}
