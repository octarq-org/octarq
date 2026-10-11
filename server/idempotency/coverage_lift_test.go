package idempotency

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) {
	return 0, errors.New("read failed")
}

func (brokenReader) Close() error {
	return nil
}

func TestCoverageLift_BodyReadError(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))

	r := httptest.NewRequest(http.MethodPost, "/api/emails/send", brokenReader{})
	r.Header.Set(HeaderKey, "key-broken-reader")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "could not read request body") {
		t.Fatalf("unexpected response body: %s", w.Body.String())
	}
}

func TestCoverageLift_CaptureWriter_ImplicitStatusOK(t *testing.T) {
	s, _ := newStore(t)
	var handlerCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// Intentionally omit WriteHeader; go straight to Write
		_, _ = w.Write([]byte(`{"status":"implicit-200"}`))
	})
	h := s.Middleware(org7)(handler)

	resp := post(t, h, "key-implicit-status", `{"req":"data"}`)
	if !handlerCalled {
		t.Fatal("handler was not called")
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.Code)
	}
}

func TestCoverageLift_StoreDatabaseErrors(t *testing.T) {
	s, gdb := newStore(t)
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("gdb.DB() failed: %v", err)
	}

	// Close the DB connection so subsequent queries fail
	_ = sqlDB.Close()

	// 1. claim with closed DB -> triggers claimError and fallback next.ServeHTTP
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))
	r := httptest.NewRequest(http.MethodPost, "/api/emails/send", strings.NewReader(`{"hello":"world"}`))
	r.Header.Set(HeaderKey, "key-db-down")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if calls.Load() != 1 {
		t.Fatalf("expected fallback handler execution (calls=1), got %d", calls.Load())
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created from fallback handler, got %d", w.Code)
	}

	// 2. commit with closed DB -> triggers error log
	s.commit(1, http.StatusOK, "application/json", []byte(`{}`), true, time.Now())

	// 3. release with closed DB -> triggers error log
	s.release(1)
}
