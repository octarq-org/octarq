package cron

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

var _ plugin.CronService = (*Engine)(nil)

type jobEntry struct {
	name     string
	spec     string
	schedule Schedule
	handler  func(ctx context.Context) error
	lastRun  time.Time
	nextRun  time.Time
	status   string // "idle" | "running" | "error"
	lastErr  string
}

// Engine implements the in-process cron scheduling engine and plugin.CronService.
type Engine struct {
	guard   Guard
	mu      sync.RWMutex
	jobs    map[string]*jobEntry
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New creates a new Engine with a Guard derived from redisURL.
func New(redisURL string) *Engine {
	return NewEngine(NewGuard(redisURL))
}

// NewEngine creates a new Engine with the specified Guard.
func NewEngine(guard Guard) *Engine {
	if guard == nil {
		guard = NewLocalGuard()
	}
	return &Engine{
		guard: guard,
		jobs:  make(map[string]*jobEntry),
	}
}

// Register registers a new cron task with the given spec and handler.
func (e *Engine) Register(name string, spec string, handler func(ctx context.Context) error) error {
	if name == "" {
		return errors.New("cron: job name cannot be empty")
	}
	if handler == nil {
		return errors.New("cron: handler cannot be nil")
	}

	sched, err := ParseStandard(spec)
	if err != nil {
		return fmt.Errorf("cron: parse spec %q: %w", spec, err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	next := sched.Next(now)

	e.jobs[name] = &jobEntry{
		name:     name,
		spec:     spec,
		schedule: sched,
		handler:  handler,
		nextRun:  next,
		status:   "idle",
	}

	return nil
}

// Unregister removes a registered cron task.
func (e *Engine) Unregister(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.jobs[name]; !exists {
		return fmt.Errorf("cron: job %q not found", name)
	}
	delete(e.jobs, name)
	return nil
}

// List returns a snapshot of all registered cron jobs sorted alphabetically by name.
func (e *Engine) List() []plugin.CronJob {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]plugin.CronJob, 0, len(e.jobs))
	for _, j := range e.jobs {
		result = append(result, plugin.CronJob{
			Name:    j.name,
			Spec:    j.spec,
			LastRun: j.lastRun,
			NextRun: j.nextRun,
			Status:  j.status,
			LastErr: j.lastErr,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// Start launches the background scheduling loop.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return errors.New("cron: engine already running")
	}
	e.running = true
	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.mu.Unlock()

	go e.run(runCtx)
	return nil
}

func (e *Engine) run(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.checkAndRun(ctx)
		}
	}
}

func (e *Engine) checkAndRun(ctx context.Context) {
	e.mu.Lock()
	now := time.Now()
	var toRun []*jobEntry

	for _, j := range e.jobs {
		if !j.nextRun.IsZero() && (now.Equal(j.nextRun) || now.After(j.nextRun)) {
			j.nextRun = j.schedule.Next(now)
			toRun = append(toRun, j)
		}
	}
	e.mu.Unlock()

	for _, j := range toRun {
		e.wg.Add(1)
		go func(entry *jobEntry) {
			defer e.wg.Done()
			e.executeJob(ctx, entry)
		}(j)
	}
}

func (e *Engine) executeJob(ctx context.Context, j *jobEntry) {
	release, ok, err := e.guard.Acquire(ctx, j.name, 10*time.Minute)
	if err != nil || !ok {
		// Guard rejected or lock already held
		return
	}
	defer release()

	e.mu.Lock()
	j.status = "running"
	j.lastRun = time.Now()
	e.mu.Unlock()

	var runErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				runErr = fmt.Errorf("panic: %v", r)
				slog.Error("cron: job panicked", "job", j.name, "panic", r)
			}
		}()
		runErr = j.handler(ctx)
	}()

	e.mu.Lock()
	defer e.mu.Unlock()

	if runErr != nil {
		j.status = "error"
		j.lastErr = runErr.Error()
		slog.Error("cron: job execution failed", "job", j.name, "err", runErr)
	} else {
		j.status = "idle"
		j.lastErr = ""
	}
}

// RunJob runs a job once immediately outside its schedule.
func (e *Engine) RunJob(ctx context.Context, name string) error {
	e.mu.RLock()
	j, exists := e.jobs[name]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("cron: job %q not found", name)
	}

	e.executeJob(ctx, j)
	e.mu.RLock()
	defer e.mu.RUnlock()
	if j.lastErr != "" {
		return errors.New(j.lastErr)
	}
	return nil
}

// Stop stops the scheduling loop and waits gracefully for currently running jobs to complete.
func (e *Engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = false
	cancel := e.cancel
	e.cancel = nil
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
