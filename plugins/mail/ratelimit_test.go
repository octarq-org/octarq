package mail

import (
	"testing"
	"time"
)

func TestRateLimiterMemory(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter("", "test", 2, time.Second)
	if !rl.allow("1.2.3.4") {
		t.Errorf("expected initial allow to be true")
	}

	rl.recordFailure("1.2.3.4")
	rl.recordFailure("1.2.3.4")

	if rl.allow("1.2.3.4") {
		t.Errorf("expected allow to be false after 2 failures")
	}

	// Different IP should still be allowed
	if !rl.allow("5.6.7.8") {
		t.Errorf("expected different IP to be allowed")
	}
}

func TestRateLimiterRedisFallback(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter("redis://127.0.0.1:58999/0", "test", 5, time.Second)
	if rl == nil || rl.store == nil {
		t.Fatal("expected non-nil rateLimiter fallback to memory store")
		return
	}
	if !rl.allow("1.2.3.4") {
		t.Errorf("expected allow to be true")
	}
}
