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
		return
	}
	rl.recordFailure("key")
	if ok, err := rl.allow("key-for-first"); !ok || err != nil {
		t.Errorf("a fresh key must be allowed, got ok=%v err=%v", ok, err)
	}
}

func TestRateLimiterAllowsUntilExhausted(t *testing.T) {
	rl := newRateLimiter("", "send", 2, time.Hour)
	if ok, err := rl.allow("ip"); !ok || err != nil {
		t.Fatalf("first allow must pass, got ok=%v err=%v", ok, err)
	}
	rl.recordFailure("ip")
	if ok, err := rl.allow("ip"); !ok || err != nil {
		t.Fatalf("second request within the limit must pass, got ok=%v err=%v", ok, err)
	}
	rl.recordFailure("ip")
	if ok, _ := rl.allow("ip"); ok {
		t.Error("third request past the limit must be refused")
	}
}

// failStore peeks with an error, standing in for a configured Redis that is
// unreachable.
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

// An unreadable counter is not permission to send. Allowing here would turn a
// counter outage into an unlimited window against the sending domain's
// reputation, so the send is refused and the caller reports the check failure.
func TestRateLimiterFailsClosedOnStoreError(t *testing.T) {
	rl := &rateLimiter{store: failStore{}, rate: limiter.Rate{Limit: 100, Period: time.Hour}}
	ok, err := rl.allow("ip")
	if ok {
		t.Fatal("a store error must refuse the send, not allow it")
	}
	if err == nil {
		t.Fatal("the store error must reach the caller so it can report the check failure")
	}
	rl.recordFailure("ip") // must not panic
}
