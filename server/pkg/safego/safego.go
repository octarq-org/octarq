package safego

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

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

func Recover(name string) {
	if r := recover(); r != nil {
		HandleRecover(name, r)
	}
}

func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

func RecoverHandler(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer Recover(name)
		next.ServeHTTP(w, r)
	})
}
