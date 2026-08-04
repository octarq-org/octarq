// Package safego provides panic-recovery helpers for goroutines so that a
// single misbehaving plugin cannot bring down the entire octarq process.
package safego

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover is intended to be deferred at the top of a long-lived goroutine:
//
//	defer safego.Recover("links.click-worker")
//
// It captures any panic (except http.ErrAbortHandler, which is re-panicked
// because net/http uses it as a sentinel for normal connection teardown),
// logs it with a full stack trace, and returns normally.
func Recover(name string) {
	if r := recover(); r != nil {
		// http.ErrAbortHandler is a sentinel net/http uses to tear down
		// the connection cleanly — swallowing it would turn a normal
		// client disconnect into a spurious 500 log entry.
		if r == http.ErrAbortHandler {
			panic(r)
		}
		slog.Error("panic recovered",
			"goroutine", name,
			"panic", r,
			"stack", string(debug.Stack()),
		)
	}
}

// Go launches fn in a new goroutine wrapped with Recover(name).
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}
