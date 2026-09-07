package cron

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Guard prevents concurrent or overlapping executions of the same job.
type Guard interface {
	Acquire(ctx context.Context, jobName string, ttl time.Duration) (release func(), ok bool, err error)
}

type redisClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
}

type distGuard struct {
	client     redisClient
	mu         sync.Mutex
	localLocks map[string]*sync.Mutex
}

// NewLocalGuard returns a Guard using in-process sync.Mutex locks only.
func NewLocalGuard() Guard {
	return &distGuard{
		localLocks: make(map[string]*sync.Mutex),
	}
}

// NewDistGuard returns a Guard backed by Redis SETNX with automatic fallback to local sync.Mutex.
func NewDistGuard(client redis.UniversalClient) Guard {
	return &distGuard{
		client:     client,
		localLocks: make(map[string]*sync.Mutex),
	}
}

// NewGuard creates a Guard based on the provided redisURL.
// If redisURL is empty or fails to connect, it falls back to NewLocalGuard.
func NewGuard(redisURL string) Guard {
	if redisURL == "" {
		return NewLocalGuard()
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Warn("cron: failed to parse redis URL, using local guard", "err", err)
		return NewLocalGuard()
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		slog.Warn("cron: redis ping failed, using local guard", "err", err)
		return NewLocalGuard()
	}

	return NewDistGuard(client)
}

func (g *distGuard) getLocalLock(jobName string) *sync.Mutex {
	g.mu.Lock()
	defer g.mu.Unlock()

	l, exists := g.localLocks[jobName]
	if !exists {
		l = &sync.Mutex{}
		g.localLocks[jobName] = l
	}
	return l
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

func (g *distGuard) Acquire(ctx context.Context, jobName string, ttl time.Duration) (func(), bool, error) {
	localLock := g.getLocalLock(jobName)
	if !localLock.TryLock() {
		// Already executing locally in this process
		return nil, false, nil
	}

	// If no Redis client is configured, local lock is sufficient
	if g.client == nil {
		var once sync.Once
		return func() {
			once.Do(localLock.Unlock)
		}, true, nil
	}

	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	key := "octarq:cron:guard:" + jobName
	token := randomToken()

	acquired, err := g.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		// Redis error: degrade gracefully to local lock
		slog.Warn("cron: redis setnx failed, degrading to local guard", "job", jobName, "err", err)
		var once sync.Once
		return func() {
			once.Do(localLock.Unlock)
		}, true, nil
	}

	if !acquired {
		// Another cluster node holds the lock
		localLock.Unlock()
		return nil, false, nil
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			defer localLock.Unlock()
			const delScript = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
			_ = g.client.Eval(context.Background(), delScript, []string{key}, token).Err()
		})
	}

	return release, true, nil
}
