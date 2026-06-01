package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tongyichu/track_server/internal/service"
)

const defaultCompanionAutoCloseSpec = "@every 10m"

// CompanionAutoClose is the scheduled fallback that ends forgotten companion sessions.
type CompanionAutoClose struct {
	service *service.CompanionService
	spec    string
	nowFunc func() time.Time
}

type CompanionAutoCloseOption func(*CompanionAutoClose)

func WithCompanionAutoCloseNowFunc(fn func() time.Time) CompanionAutoCloseOption {
	return func(j *CompanionAutoClose) {
		if fn != nil {
			j.nowFunc = fn
		}
	}
}

func NewCompanionAutoClose(companionSvc *service.CompanionService, opts ...CompanionAutoCloseOption) *CompanionAutoClose {
	job := &CompanionAutoClose{
		service: companionSvc,
		spec:    defaultCompanionAutoCloseSpec,
		nowFunc: time.Now,
	}
	for _, opt := range opts {
		opt(job)
	}
	return job
}

func (j *CompanionAutoClose) Name() string { return "companion_session_autoclose" }

func (j *CompanionAutoClose) Spec() string { return j.spec }

func (j *CompanionAutoClose) Run(ctx context.Context) error {
	if j.service == nil {
		return fmt.Errorf("companion_session_autoclose: service is nil")
	}
	result, err := j.service.AutoCloseInactiveSessions(ctx, j.nowFunc())
	if err != nil {
		return err
	}
	log.Printf("[scheduler] companion_session_autoclose: scanned=%d closed=%d", result.Scanned, result.Closed)
	return nil
}
