package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fakeJob 是测试用的 Job 实现。
type fakeJob struct {
	name    string
	spec    string
	runs    int32
	runErr  error
	doPanic bool
}

func (j *fakeJob) Name() string { return j.name }
func (j *fakeJob) Spec() string { return j.spec }
func (j *fakeJob) Run(ctx context.Context) error {
	atomic.AddInt32(&j.runs, 1)
	if j.doPanic {
		panic("boom")
	}
	return j.runErr
}

func TestScheduler_RegisterDuplicateName(t *testing.T) {
	s := New()
	a := &fakeJob{name: "dup", spec: "@every 1m"}
	b := &fakeJob{name: "dup", spec: "@every 1m"}
	if err := s.Register(a); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := s.Register(b); err == nil {
		t.Fatalf("expected duplicate name error")
	}
}

func TestScheduler_RegisterEmptyName(t *testing.T) {
	s := New()
	if err := s.Register(&fakeJob{name: "", spec: "@every 1m"}); err == nil {
		t.Fatalf("expected empty name error")
	}
}

func TestScheduler_RegisterInvalidSpec(t *testing.T) {
	s := New()
	if err := s.Register(&fakeJob{name: "bad", spec: "not a valid cron"}); err == nil {
		t.Fatalf("expected invalid spec error")
	}
}

func TestScheduler_RegisterNilJob(t *testing.T) {
	s := New()
	if err := s.Register(nil); err == nil {
		t.Fatalf("expected nil job error")
	}
}

func TestScheduler_StartTriggersRun(t *testing.T) {
	s := New()
	job := &fakeJob{name: "tick", spec: "@every 1s"}
	if err := s.Register(job); err != nil {
		t.Fatalf("register: %v", err)
	}
	s.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	}()
	time.Sleep(2500 * time.Millisecond)
	if got := atomic.LoadInt32(&job.runs); got < 2 {
		t.Fatalf("expected job to run at least twice, got %d", got)
	}
}

func TestScheduler_PanicRecovered(t *testing.T) {
	s := New()
	job := &fakeJob{name: "boom", spec: "@every 1s", doPanic: true}
	if err := s.Register(job); err != nil {
		t.Fatalf("register: %v", err)
	}
	s.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	}()
	time.Sleep(2500 * time.Millisecond)
	if got := atomic.LoadInt32(&job.runs); got < 2 {
		t.Fatalf("expected scheduler to keep ticking after panic, runs=%d", got)
	}
}

func TestScheduler_RunErrorDoesNotBreak(t *testing.T) {
	s := New()
	job := &fakeJob{name: "err", spec: "@every 1s", runErr: errors.New("oops")}
	if err := s.Register(job); err != nil {
		t.Fatalf("register: %v", err)
	}
	s.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	}()
	time.Sleep(2500 * time.Millisecond)
	if got := atomic.LoadInt32(&job.runs); got < 2 {
		t.Fatalf("expected scheduler to keep ticking after run error, runs=%d", got)
	}
}

func TestScheduler_JobsReturnsCopy(t *testing.T) {
	s := New()
	if err := s.Register(&fakeJob{name: "a", spec: "@every 1m"}, &fakeJob{name: "b", spec: "@every 1m"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	jobs := s.Jobs()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	jobs[0] = nil
	if s.Jobs()[0] == nil {
		t.Fatalf("Jobs() should return a copy, internal state must remain intact")
	}
}
