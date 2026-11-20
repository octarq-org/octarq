package cron

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Guard interface {
	Acquire(ctx context.Context, jobName string, ttl time.Duration) (release func(), ok bool, err error)
}

type failClosedGuard struct {
	err error
}

func (g *failClosedGuard) Acquire(ctx context.Context, jobName string, ttl time.Duration) (func(), bool, error) {
	return nil, false, g.err
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

func NewLocalGuard() Guard {
	return &distGuard{
		localLocks: make(map[string]*sync.Mutex),
	}
}

func NewDistGuard(client redis.UniversalClient) Guard {
	return &distGuard{
		client:     client,
		localLocks: make(map[string]*sync.Mutex),
	}
}

func NewGuard(redisURL string) Guard {
	if redisURL == "" {
		return NewLocalGuard()
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Error("cron: failed to parse redis URL, failing closed", "err", err)
		return &failClosedGuard{err: fmt.Errorf("cron: invalid redis url: %w", err)}
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		slog.Error("cron: redis ping failed, failing closed", "err", err)
		return &failClosedGuard{err: fmt.Errorf("cron: redis ping failed: %w", err)}
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
		return nil, false, nil
	}

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
		localLock.Unlock()
		slog.Error("cron: redis setnx failed, failing closed", "job", jobName, "err", err)
		return nil, false, fmt.Errorf("cron: redis setnx failed: %w", err)
	}

	if !acquired {
		localLock.Unlock()
		return nil, false, nil
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			defer localLock.Unlock()
			const delScript = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
			if err := g.client.Eval(context.Background(), delScript, []string{key}, token).Err(); err != nil {
				slog.Warn("cron: failed to release redis distributed lock", "job", jobName, "err", err)
			}
		})
	}

	return release, true, nil
}
