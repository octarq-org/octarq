package eventbus

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// ReactorRegistry manages declarative EventReactor registration and lifecycle.
type ReactorRegistry struct {
	mu       sync.RWMutex
	reactors []plugin.EventReactor
}

// NewReactorRegistry creates a new isolated ReactorRegistry instance.
func NewReactorRegistry() *ReactorRegistry {
	return &ReactorRegistry{
		reactors: make([]plugin.EventReactor, 0),
	}
}

var defaultReactorRegistry = NewReactorRegistry()

// Register registers an EventReactor to the registry.
// Returns an error if the reactor is nil, defines no events, contains invalid keys,
// or has already been registered.
func (reg *ReactorRegistry) Register(r plugin.EventReactor) error {
	if r == nil {
		return errors.New("event reactor cannot be nil")
	}
	events := r.Events()
	if len(events) == 0 {
		return errors.New("event reactor must declare at least one event key in Events()")
	}
	for _, k := range events {
		if strings.TrimSpace(k) == "" {
			return errors.New("event key cannot be empty")
		}
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	for _, existing := range reg.reactors {
		if existing == r {
			return fmt.Errorf("event reactor %T already registered", r)
		}
	}

	reg.reactors = append(reg.reactors, r)
	return nil
}

// Reactors returns a copy of all registered EventReactors.
func (reg *ReactorRegistry) Reactors() []plugin.EventReactor {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]plugin.EventReactor, len(reg.reactors))
	copy(out, reg.reactors)
	return out
}

// Reset clears all registered reactors in this registry.
func (reg *ReactorRegistry) Reset() {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.reactors = nil
}

// RegisterReactor registers an EventReactor into the default global registry.
func RegisterReactor(r plugin.EventReactor) error {
	return defaultReactorRegistry.Register(r)
}

// ResetReactors clears all registered reactors from the default global registry.
func ResetReactors() {
	defaultReactorRegistry.Reset()
}

type reactorState struct {
	mu       sync.Mutex
	lastFire map[string]time.Time
	ops      uint64
}

func (s *reactorState) shouldDebounce(entityKey string, minInterval time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.ops++
	// Amortized eviction sweep every 1,000 operations when map exceeds threshold to avoid O(N) overhead on every call under lock
	if s.ops%1000 == 0 && len(s.lastFire) > 10000 {
		for k, t := range s.lastFire {
			if now.Sub(t) > minInterval*2 {
				delete(s.lastFire, k)
			}
		}
	}
	if last, ok := s.lastFire[entityKey]; ok {
		if now.Sub(last) < minInterval {
			return true
		}
	}
	s.lastFire[entityKey] = now
	return false
}

type reactorTask struct {
	reactor   plugin.EventReactor
	env       plugin.Envelope
	entityKey string
}

func hashEntityKey(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// Start launches the subscriber and worker pool for all registered reactors in this registry.
// Concurrency defaults to GOMAXPROCS (or 8 if GOMAXPROCS < 8) when concurrency <= 0.
// It returns an idempotent stop function for graceful shutdown.
func (reg *ReactorRegistry) Start(ctx context.Context, concurrency int) func() {
	if concurrency <= 0 {
		concurrency = runtime.GOMAXPROCS(0)
		if concurrency < 8 {
			concurrency = 8
		}
	}

	reactors := reg.Reactors()
	if len(reactors) == 0 {
		return func() {}
	}

	reactorsByKey := make(map[string][]plugin.EventReactor)
	reactorStates := make(map[plugin.EventReactor]*reactorState, len(reactors))
	keySet := make(map[string]struct{})

	for _, r := range reactors {
		reactorStates[r] = &reactorState{
			lastFire: make(map[string]time.Time),
		}
		for _, ev := range r.Events() {
			ev = strings.TrimSpace(ev)
			if ev != "" {
				keySet[ev] = struct{}{}
				reactorsByKey[ev] = append(reactorsByKey[ev], r)
			}
		}
	}

	if len(keySet) == 0 {
		return func() {}
	}

	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}

	subCh, cancelSub := Subscribe(SubscribeOpts{
		Keys:       keys,
		BufferSize: 1024,
	})

	runnerCtx, runnerCancel := context.WithCancel(ctx)
	shardQueues := make([]chan reactorTask, concurrency)
	for i := 0; i < concurrency; i++ {
		shardQueues[i] = make(chan reactorTask, 256)
	}

	var wg sync.WaitGroup
	var rr uint32

	// Start worker goroutines (1 worker per shard queue for strict per-entity serialization)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(shardID int, queue <-chan reactorTask) {
			defer wg.Done()
			for {
				select {
				case <-runnerCtx.Done():
					return
				case task, ok := <-queue:
					if !ok {
						return
					}
					if runnerCtx.Err() != nil {
						return
					}
					executeReactor(runnerCtx, task.reactor, task.env, task.entityKey)
				}
			}
		}(i, shardQueues[i])
	}

	// Dispatcher goroutine: consumes from event spine subscriber channel and fans out to worker shards
	wg.Add(1)
	go func() {
		defer func() {
			for _, q := range shardQueues {
				close(q)
			}
			wg.Done()
		}()

		for {
			select {
			case <-runnerCtx.Done():
				return
			case env, ok := <-subCh:
				if !ok {
					return
				}
				matchedReactors := reactorsByKey[env.Key]
				for _, r := range matchedReactors {
					// Extract EntityKey
					entityKey := env.EntityKey
					if ekr, ok := r.(plugin.EntityKeyReactor); ok {
						entityKey = ekr.EntityKey(env)
					}

					// Per-reactor debounce check
					if dr, ok := r.(plugin.DebounceReactor); ok {
						minInterval := dr.MinInterval()
						if minInterval > 0 && entityKey != "" {
							state := reactorStates[r]
							if state.shouldDebounce(entityKey, minInterval) {
								atomic.AddUint64(&droppedCnt, 1)
								continue
							}
						}
					}

					// Select shard for serialization
					var shardID int
					if entityKey != "" {
						shardID = int(hashEntityKey(entityKey) % uint32(concurrency))
					} else {
						shardID = int(atomic.AddUint32(&rr, 1) % uint32(concurrency))
					}

					task := reactorTask{
						reactor:   r,
						env:       env,
						entityKey: entityKey,
					}

					select {
					case shardQueues[shardID] <- task:
					case <-runnerCtx.Done():
						return
					default:
						atomic.AddUint64(&droppedCnt, 1)
					}
				}
			}
		}
	}()

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			runnerCancel()
			cancelSub()
			wg.Wait()
		})
	}

	go func() {
		select {
		case <-ctx.Done():
			stop()
		case <-runnerCtx.Done():
			return
		}
	}()

	return stop
}

func executeReactor(ctx context.Context, r plugin.EventReactor, env plugin.Envelope, entityKey string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("eventbus: reactor %T panicked on event %s (entity %s): %v", r, env.Key, entityKey, rec)
		}
	}()
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := r.React(execCtx, env); err != nil {
		// Best-effort execution: log error and do not block the event spine.
		// For guaranteed transactional retries across restarts, background queues (asynq) are used.
		log.Printf("eventbus: reactor %T failed on event %s (entity %s): %v", r, env.Key, entityKey, err)
	}
}

// StartReactors starts reactors registered in the default global registry.
func StartReactors(ctx context.Context, concurrency int) func() {
	return defaultReactorRegistry.Start(ctx, concurrency)
}
