package cron

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

type fakeStore struct {
	mu   sync.Mutex
	data map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{data: map[string]string{}}
}

func (s *fakeStore) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *fakeStore) Set(key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func TestEngine_AppliesStoredSpecAndEnabledOnRegister(t *testing.T) {
	store := newFakeStore()
	store.data["cron.pt.tick.spec"] = "*/15 * * * *"
	store.data["cron.pt.tick.enabled"] = "false"

	engine := NewEngine(nil)
	engine.SetStore(store)
	if err := engine.Register("pt.tick", "*/5 * * * *", func(ctx context.Context) error { return nil }); err != nil {
		t.Fatalf("register: %v", err)
	}

	jobs := engine.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Spec != "*/15 * * * *" || jobs[0].DefaultSpec != "*/5 * * * *" {
		t.Fatalf("spec override not applied: %+v", jobs[0])
	}
	if jobs[0].Enabled {
		t.Fatal("stored enabled=false was ignored")
	}
}

func TestEngine_DisabledJobIsNotScheduled(t *testing.T) {
	engine := NewEngine(nil)
	engine.SetStore(newFakeStore())

	var runs int
	if err := engine.Register("idle_job", "* * * * *", func(ctx context.Context) error {
		runs++
		return nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := engine.SetEnabled("idle_job", false); err != nil {
		t.Fatalf("disable: %v", err)
	}

	engine.mu.Lock()
	engine.jobs["idle_job"].nextRun = time.Now().Add(-time.Minute)
	engine.mu.Unlock()

	engine.checkAndRun(context.Background())
	time.Sleep(100 * time.Millisecond)
	if runs != 0 {
		t.Fatalf("disabled job ran %d times", runs)
	}
}

func TestEngine_SetSpecPersistsAndReschedules(t *testing.T) {
	store := newFakeStore()
	engine := NewEngine(nil)
	engine.SetStore(store)
	if err := engine.Register("job", "*/5 * * * *", func(ctx context.Context) error { return nil }); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := engine.SetSpec("job", "not a cron"); err == nil {
		t.Fatal("expected error for invalid spec")
	}
	if err := engine.SetSpec("job", "0 3 * * *"); err != nil {
		t.Fatalf("set spec: %v", err)
	}
	if got, _ := store.Get("cron.job.spec"); got != "0 3 * * *" {
		t.Fatalf("spec not persisted: %q", got)
	}
	jobs := engine.List()
	if jobs[0].Spec != "0 3 * * *" {
		t.Fatalf("spec not applied: %+v", jobs[0])
	}
}

func TestEngine_StartJob(t *testing.T) {
	engine := NewEngine(nil)
	done := make(chan struct{})
	if err := engine.Register("job", "*/5 * * * *", func(ctx context.Context) error {
		close(done)
		return nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := engine.StartJob("missing"); !errors.Is(err, plugin.ErrJobNotFound) {
		t.Fatalf("missing job: got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = engine.Stop(context.Background()) }()

	if err := engine.StartJob("job"); err != nil {
		t.Fatalf("start job: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run")
	}
}

func TestEngine_StartJobBusy(t *testing.T) {
	engine := NewEngine(&rejectingGuard{})
	if err := engine.Register("job", "*/5 * * * *", func(ctx context.Context) error { return nil }); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = engine.Stop(context.Background()) }()

	if err := engine.StartJob("job"); !errors.Is(err, plugin.ErrJobBusy) {
		t.Fatalf("expected ErrJobBusy, got %v", err)
	}
}
