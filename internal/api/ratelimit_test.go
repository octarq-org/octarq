package api

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	// Limit: 2 failures per 100ms
	rl := newRateLimiter(2, 100*time.Millisecond)

	// 1. Initially allowed
	if !rl.allow("1.1.1.1") {
		t.Error("expected initial allow to be true")
	}

	// 2. 1st failure
	rl.recordFailure("1.1.1.1")
	if !rl.allow("1.1.1.1") {
		t.Error("expected allow to be true after 1st failure")
	}

	// 3. 2nd failure
	rl.recordFailure("1.1.1.1")
	if rl.allow("1.1.1.1") {
		t.Error("expected allow to be false after 2nd failure")
	}

	// 4. Reset ip
	rl.reset("1.1.1.1")
	if !rl.allow("1.1.1.1") {
		t.Error("expected allow to be true after reset")
	}

	// 5. Cleanup / Window expiry
	rl.recordFailure("2.2.2.2")
	rl.recordFailure("2.2.2.2")
	if rl.allow("2.2.2.2") {
		t.Error("expected allow to be false for 2.2.2.2")
	}

	time.Sleep(150 * time.Millisecond)
	// After window duration, it should be allowed
	if !rl.allow("2.2.2.2") {
		t.Error("expected allow to be true after window elapsed")
	}

	// Double check cleanup runs and cleans up the client map
	rl.recordFailure("3.3.3.3")
	time.Sleep(150 * time.Millisecond)
	// Running allow/recordFailure on another IP triggers lazy cleanup of 3.3.3.3
	rl.allow("4.4.4.4")
	rl.mu.Lock()
	_, exists := rl.clients["3.3.3.3"]
	rl.mu.Unlock()
	if exists {
		t.Error("expected 3.3.3.3 to be cleaned up after window elapsed")
	}
}
