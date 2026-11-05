package mail

import (
	"testing"
	"time"
)

func TestRateLimiterMemory(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter("", "test", 2, time.Second)
	if ok, err := rl.allow("1.2.3.4"); !ok || err != nil {
		t.Errorf("expected initial allow to be true, got ok=%v err=%v", ok, err)
	}

	rl.recordFailure("1.2.3.4")
	rl.recordFailure("1.2.3.4")

	if ok, _ := rl.allow("1.2.3.4"); ok {
		t.Errorf("expected allow to be false after 2 failures")
	}

	// Different IP should still be allowed
	if ok, err := rl.allow("5.6.7.8"); !ok || err != nil {
		t.Errorf("expected different IP to be allowed, got ok=%v err=%v", ok, err)
	}
}

func TestRateLimiterRedisFallback(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter("redis://127.0.0.1:58999/0", "test", 5, time.Second)
	if rl == nil || rl.store == nil {
		t.Fatal("expected non-nil rateLimiter fallback to memory store")
		return
	}
	if ok, err := rl.allow("1.2.3.4"); !ok || err != nil {
		t.Errorf("expected allow to be true, got ok=%v err=%v", ok, err)
	}
}
