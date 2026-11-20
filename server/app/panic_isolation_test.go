package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatedMux_PanicReturns500(t *testing.T) {
	real := http.NewServeMux()
	gm := &gatedMux{
		real:   real,
		plugin: "crash-test",
		enabled: func(_ *http.Request, _ string) (bool, bool) {
			return true, true
		},
	}
	gm.HandleFunc("/api/boom", func(w http.ResponseWriter, _ *http.Request) {
		panic("plugin exploded")
	})

	rec := httptest.NewRecorder()
	real.ServeHTTP(rec, httptest.NewRequest("GET", "/api/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if body["error"] != "internal server error" {
		t.Fatalf("unexpected error body: %v", body)
	}
}

func TestGatedMux_ErrAbortHandlerRepanics(t *testing.T) {
	real := http.NewServeMux()
	gm := &gatedMux{
		real:   real,
		plugin: "abort-test",
		enabled: func(_ *http.Request, _ string) (bool, bool) {
			return true, true
		},
	}
	gm.HandleFunc("/api/abort", func(w http.ResponseWriter, _ *http.Request) {
		panic(http.ErrAbortHandler)
	})

	caught := false
	func() {
		defer func() {
			if r := recover(); r == http.ErrAbortHandler {
				caught = true
			}
		}()
		rec := httptest.NewRecorder()
		real.ServeHTTP(rec, httptest.NewRequest("GET", "/api/abort", nil))
	}()
	if !caught {
		t.Fatal("http.ErrAbortHandler was swallowed instead of re-panicked")
	}
}

func TestGatedMux_PanicAfterPartialWrite(t *testing.T) {
	real := http.NewServeMux()
	gm := &gatedMux{
		real:   real,
		plugin: "partial-write",
		enabled: func(_ *http.Request, _ string) (bool, bool) {
			return true, true
		},
	}
	gm.HandleFunc("/api/partial", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("mid-write panic")
	})

	rec := httptest.NewRecorder()
	real.ServeHTTP(rec, httptest.NewRequest("GET", "/api/partial", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (first WriteHeader wins), got %d", rec.Code)
	}
}
