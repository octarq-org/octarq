package mail

import (
	"context"
	"testing"
	"time"

	"github.com/ulule/limiter/v3"
)

func TestRateLimiterFallsBackToMemoryOnBadRedisURL(t *testing.T) {
	rl := newRateLimiter("not-a-url://", "send", 100, time.Hour)
	if rl.store == nil {
		t.Fatal("store must not be nil")
	}
	rl.recordFailure("key")
	if !rl.allow("key-for-first") {
		t.Error("a fresh key must be allowed")
	}
}

func TestRateLimiterAllowsUntilExhausted(t *testing.T) {
	rl := newRateLimiter("", "send", 2, time.Hour)
	if !rl.allow("ip") {
		t.Fatal("first allow must pass")
	}
	rl.recordFailure("ip")
	if !rl.allow("ip") {
		t.Fatal("second request within the limit must pass")
	}
	rl.recordFailure("ip")
	if rl.allow("ip") {
		t.Error("third request past the limit must be refused")
	}
}

// failStore peeks with an error so allow() must fail-soft to true.
type failStore struct{}

func (failStore) Get(_ context.Context, _ string, _ limiter.Rate) (limiter.Context, error) {
	return limiter.Context{}, errBoom
}
func (failStore) Peek(_ context.Context, _ string, _ limiter.Rate) (limiter.Context, error) {
	return limiter.Context{}, errBoom
}
func (failStore) Reset(_ context.Context, _ string, _ limiter.Rate) (limiter.Context, error) {
	return limiter.Context{}, errBoom
}
func (failStore) Increment(_ context.Context, _ string, _ int64, _ limiter.Rate) (limiter.Context, error) {
	return limiter.Context{}, errBoom
}

func TestRateLimiterFailsSoftOnStoreError(t *testing.T) {
	rl := &rateLimiter{store: failStore{}}
	if !rl.allow("ip") {
		t.Error("store errors must fail open (allow), not block mail")
	}
	rl.recordFailure("ip") // must not panic
}
