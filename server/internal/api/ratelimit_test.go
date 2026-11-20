package api

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl, err := newRateLimiter("", "test", 2, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("memory limiter: %v", err)
	}

	if !rl.allow("1.1.1.1") {
		t.Error("expected initial allow to be true")
	}

	rl.recordFailure("1.1.1.1")
	if !rl.allow("1.1.1.1") {
		t.Error("expected allow to be true after 1st failure")
	}

	rl.recordFailure("1.1.1.1")
	if rl.allow("1.1.1.1") {
		t.Error("expected allow to be false after 2nd failure")
	}

	rl.reset("1.1.1.1")
	if !rl.allow("1.1.1.1") {
		t.Error("expected allow to be true after reset")
	}

	rl.recordFailure("2.2.2.2")
	rl.recordFailure("2.2.2.2")
	if rl.allow("2.2.2.2") {
		t.Error("expected allow to be false for 2.2.2.2")
	}

	time.Sleep(150 * time.Millisecond)
	if !rl.allow("2.2.2.2") {
		t.Error("expected allow to be true after window elapsed")
	}
}

func mustRateLimiter(t *testing.T, redisURL, prefix string, limit int, window time.Duration) *rateLimiter {
	t.Helper()
	rl, err := newRateLimiter(redisURL, prefix, limit, window)
	if err != nil {
		t.Fatalf("newRateLimiter(%q): %v", redisURL, err)
	}
	return rl
}
