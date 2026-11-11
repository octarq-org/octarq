// Package safego provides panic-recovery helpers for goroutines and HTTP handlers
// so that a single misbehaving component or plugin cannot bring down the entire process.
package safego

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// HandleRecover handles a non-nil recovered panic value. If r is http.ErrAbortHandler,
// it re-panics to preserve HTTP connection teardown. Otherwise it logs the panic.
func HandleRecover(name string, r any) {
	if r == nil {
		return
	}
	if r == http.ErrAbortHandler {
		panic(r)
	}
	slog.Error("panic recovered",
		"goroutine", name,
		"panic", r,
		"stack", string(debug.Stack()),
	)
}

// Recover is intended to be deferred at the top of a long-lived goroutine or request handler:
//
//	defer safego.Recover("links.click-worker")
//
// It captures any panic (except http.ErrAbortHandler, which is re-panicked
// because net/http uses it as a sentinel for normal connection teardown),
// logs it with a full stack trace, and returns normally.
func Recover(name string) {
	if r := recover(); r != nil {
		HandleRecover(name, r)
	}
}

// Go launches fn in a new goroutine wrapped with Recover(name).
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

// RecoverHandler wraps next in an http.Handler that recovers any panic using Recover(name).
func RecoverHandler(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer Recover(name)
		next.ServeHTTP(w, r)
	})
}
