package cron

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

var _ plugin.CronService = (*Engine)(nil)

type jobEntry struct {
	name        string
	spec        string
	defaultSpec string
	schedule    Schedule
	handler     func(ctx context.Context) error
	enabled     bool
	lastRun     time.Time
	nextRun     time.Time
	status      string
	lastErr     string
}

type SettingsStore interface {
	Get(key string) (string, bool)
	Set(key string, value string) error
}

const jobRunTTL = 10 * time.Minute

type Engine struct {
	guard   Guard
	store   SettingsStore
	mu      sync.RWMutex
	jobs    map[string]*jobEntry
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func New(redisURL string) *Engine {
	return NewEngine(NewGuard(redisURL))
}

func NewEngine(guard Guard) *Engine {
	if guard == nil {
		guard = NewLocalGuard()
	}
	return &Engine{
		guard: guard,
		jobs:  make(map[string]*jobEntry),
	}
}

func (e *Engine) SetStore(store SettingsStore) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.store = store
}

func (e *Engine) Register(name string, spec string, handler func(ctx context.Context) error) error {
	if name == "" {
		return errors.New("cron: job name cannot be empty")
	}
	if handler == nil {
		return errors.New("cron: handler cannot be nil")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	entry := &jobEntry{
		name:        name,
		spec:        spec,
		defaultSpec: spec,
		handler:     handler,
		enabled:     true,
		status:      "idle",
	}
	e.applyStoredSettings(entry)
	sched, err := ParseStandard(entry.spec)
	if err != nil {
		return fmt.Errorf("cron: parse spec %q: %w", entry.spec, err)
	}
	entry.schedule = sched
	entry.nextRun = sched.Next(time.Now())
	e.jobs[name] = entry

	return nil
}

func (e *Engine) applyStoredSettings(entry *jobEntry) {
	if e.store == nil {
		return
	}
	if stored, ok := e.store.Get(specKey(entry.name)); ok && stored != "" {
		if _, err := ParseStandard(stored); err == nil {
			entry.spec = stored
		}
	}
	if stored, ok := e.store.Get(enabledKey(entry.name)); ok {
		entry.enabled = stored != "false"
	}
}

func specKey(name string) string    { return "cron." + name + ".spec" }
func enabledKey(name string) string { return "cron." + name + ".enabled" }

func (e *Engine) Unregister(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.jobs[name]; !exists {
		return fmt.Errorf("cron: job %q not found", name)
	}
	delete(e.jobs, name)
	return nil
}

func (e *Engine) List() []plugin.CronJob {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]plugin.CronJob, 0, len(e.jobs))
	for _, j := range e.jobs {
		result = append(result, plugin.CronJob{
			Name:        j.name,
			Spec:        j.spec,
			DefaultSpec: j.defaultSpec,
			Enabled:     j.enabled,
			LastRun:     j.lastRun,
			NextRun:     j.nextRun,
			Status:      j.status,
			LastErr:     j.lastErr,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

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
		if !j.enabled {
			continue
		}
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

func (e *Engine) executeJob(ctx context.Context, j *jobEntry) error {
	release, ok, err := e.guard.Acquire(ctx, j.name, jobRunTTL)
	if err != nil {
		e.mu.Lock()
		j.status = "error"
		j.lastErr = err.Error()
		e.mu.Unlock()
		slog.Error("cron: guard acquire failed", "job", j.name, "err", err)
		return err
	}
	if !ok {
		return plugin.ErrJobBusy
	}
	defer release()

	e.runHandler(ctx, j)
	e.mu.RLock()
	defer e.mu.RUnlock()
	if j.lastErr != "" {
		return errors.New(j.lastErr)
	}
	return nil
}

func (e *Engine) runHandler(ctx context.Context, j *jobEntry) {
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

func (e *Engine) RunJob(ctx context.Context, name string) error {
	e.mu.RLock()
	j, exists := e.jobs[name]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("%w: %q", plugin.ErrJobNotFound, name)
	}
	return e.executeJob(ctx, j)
}

func (e *Engine) StartJob(name string) error {
	e.mu.RLock()
	j, exists := e.jobs[name]
	running := e.running
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("%w: %q", plugin.ErrJobNotFound, name)
	}
	if !running {
		return errors.New("cron: engine is not running")
	}

	ctx, cancel := context.WithTimeout(context.Background(), jobRunTTL)
	release, ok, err := e.guard.Acquire(ctx, name, jobRunTTL)
	if err != nil {
		cancel()
		return err
	}
	if !ok {
		cancel()
		return fmt.Errorf("%w: %q", plugin.ErrJobBusy, name)
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancel()
		defer release()
		e.runHandler(ctx, j)
	}()
	return nil
}

func (e *Engine) SetEnabled(name string, enabled bool) error {
	if e.store == nil {
		return errors.New("cron: settings store is not configured")
	}

	e.mu.Lock()
	j, exists := e.jobs[name]
	e.mu.Unlock()
	if !exists {
		return fmt.Errorf("%w: %q", plugin.ErrJobNotFound, name)
	}

	value := "true"
	if !enabled {
		value = "false"
	}
	if err := e.store.Set(enabledKey(name), value); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	j.enabled = enabled
	return nil
}

func (e *Engine) SetSpec(name string, spec string) error {
	sched, err := ParseStandard(spec)
	if err != nil {
		return fmt.Errorf("cron: parse spec %q: %w", spec, err)
	}
	if e.store == nil {
		return errors.New("cron: settings store is not configured")
	}

	e.mu.Lock()
	j, exists := e.jobs[name]
	e.mu.Unlock()
	if !exists {
		return fmt.Errorf("%w: %q", plugin.ErrJobNotFound, name)
	}

	if err := e.store.Set(specKey(name), spec); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	j.spec = spec
	j.schedule = sched
	j.nextRun = sched.Next(time.Now())
	return nil
}

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
