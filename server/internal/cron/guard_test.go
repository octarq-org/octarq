package cron

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type mockRedisClient struct {
	setNXFunc func(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	evalFunc  func(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
}

func (m *mockRedisClient) SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
	if m.setNXFunc != nil {
		return m.setNXFunc(ctx, key, value, expiration)
	}
	cmd := redis.NewBoolCmd(ctx)
	cmd.SetVal(true)
	return cmd
}

func (m *mockRedisClient) Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
	if m.evalFunc != nil {
		return m.evalFunc(ctx, script, keys, args...)
	}
	cmd := redis.NewCmd(ctx)
	cmd.SetVal(int64(1))
	return cmd
}

func TestLocalGuard(t *testing.T) {
	ctx := context.Background()
	guard := NewLocalGuard()

	rel1, ok1, err := guard.Acquire(ctx, "job1", time.Minute)
	if err != nil || !ok1 || rel1 == nil {
		t.Fatalf("expected acquire success, got ok=%v, err=%v", ok1, err)
	}

	// Concurrent acquire for same job should fail
	rel2, ok2, err := guard.Acquire(ctx, "job1", time.Minute)
	if err != nil || ok2 || rel2 != nil {
		t.Fatalf("expected acquire fail for already locked job, got ok=%v", ok2)
	}

	// Another job can acquire
	rel3, ok3, err := guard.Acquire(ctx, "job2", time.Minute)
	if err != nil || !ok3 || rel3 == nil {
		t.Fatalf("expected acquire success for job2, got ok=%v", ok3)
	}
	rel3()

	// Release rel1
	rel1()
	// Idempotent release
	rel1()

	// Now job1 can acquire again
	rel4, ok4, err := guard.Acquire(ctx, "job1", time.Minute)
	if err != nil || !ok4 || rel4 == nil {
		t.Fatalf("expected re-acquire success after release, got ok=%v", ok4)
	}
	rel4()
}

func TestNewGuard_Fallbacks(t *testing.T) {
	// Empty URL
	g1 := NewGuard("")
	if g1 == nil {
		t.Fatal("expected non-nil guard for empty url")
	}

	// NewDistGuard constructor
	gDist := NewDistGuard(nil)
	if gDist == nil {
		t.Fatal("expected non-nil dist guard")
	}

	// Invalid URL
	g2 := NewGuard("not a valid url :///")
	if g2 == nil {
		t.Fatal("expected non-nil guard for invalid url")
	}

	// Non-existent redis host
	g3 := NewGuard("redis://127.0.0.1:58999/0")
	if g3 == nil {
		t.Fatal("expected fallback guard when redis down")
	}
}

func TestDistGuard_RedisSuccess(t *testing.T) {
	ctx := context.Background()
	evalCalled := false
	mock := &mockRedisClient{
		setNXFunc: func(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
			cmd := redis.NewBoolCmd(ctx)
			cmd.SetVal(true)
			return cmd
		},
		evalFunc: func(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
			evalCalled = true
			cmd := redis.NewCmd(ctx)
			cmd.SetVal(int64(1))
			return cmd
		},
	}

	guard := &distGuard{
		client:     mock,
		localLocks: make(map[string]*sync.Mutex),
	}

	rel, ok, err := guard.Acquire(ctx, "cluster_job", 0) // test ttl <= 0 default
	if err != nil || !ok || rel == nil {
		t.Fatalf("expected acquire success, got ok=%v, err=%v", ok, err)
	}

	rel()
	if !evalCalled {
		t.Fatalf("expected eval to be called on release")
	}
}

func TestDistGuard_RedisLockedByAnotherNode(t *testing.T) {
	ctx := context.Background()
	mock := &mockRedisClient{
		setNXFunc: func(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
			cmd := redis.NewBoolCmd(ctx)
			cmd.SetVal(false) // key already exists
			return cmd
		},
	}

	guard := &distGuard{
		client:     mock,
		localLocks: make(map[string]*sync.Mutex),
	}

	rel, ok, err := guard.Acquire(ctx, "cluster_job", time.Minute)
	if err != nil || ok || rel != nil {
		t.Fatalf("expected acquire failure when locked in redis, got ok=%v", ok)
	}

	// Verify local lock was released so future attempts can re-try
	mock.setNXFunc = func(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
		cmd := redis.NewBoolCmd(ctx)
		cmd.SetVal(true)
		return cmd
	}
	rel2, ok2, err := guard.Acquire(ctx, "cluster_job", time.Minute)
	if err != nil || !ok2 || rel2 == nil {
		t.Fatalf("expected acquire success after node release, got ok=%v", ok2)
	}
	rel2()
}

func TestDistGuard_RedisErrorDegradesToLocal(t *testing.T) {
	ctx := context.Background()
	mock := &mockRedisClient{
		setNXFunc: func(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
			cmd := redis.NewBoolCmd(ctx)
			cmd.SetErr(errors.New("redis connection reset"))
			return cmd
		},
	}

	guard := &distGuard{
		client:     mock,
		localLocks: make(map[string]*sync.Mutex),
	}

	rel, ok, err := guard.Acquire(ctx, "cluster_job", time.Minute)
	if err != nil || !ok || rel == nil {
		t.Fatalf("expected degrade to local lock on redis error, got ok=%v, err=%v", ok, err)
	}

	// While degraded lock is held, local re-acquire fails
	_, ok2, _ := guard.Acquire(ctx, "cluster_job", time.Minute)
	if ok2 {
		t.Fatalf("expected second acquire to fail while degraded lock is held")
	}

	rel()
}
