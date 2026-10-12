package idempotency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// TestHumaMiddlewareIdempotency verifies that huma operations wrapped with HumaMiddleware
// de-duplicate requests and do not execute the underlying huma handler twice.
func TestHumaMiddlewareIdempotency(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1.0.0"))

	type SendMailInput struct {
		Ctx  huma.Context
		Body struct {
			To []string `json:"to"`
		}
	}
	type SendMailOutput struct {
		Body struct {
			Sent  bool `json:"sent"`
			Count int  `json:"count"`
		}
	}

	idemMw := HumaMiddleware(s.Middleware(org7))
	huma.Register(api, huma.Operation{
		Method:      http.MethodPost,
		Path:        "/api/emails/send",
		Middlewares: huma.Middlewares{idemMw},
	}, func(ctx context.Context, input *SendMailInput) (*SendMailOutput, error) {
		cnt := int(calls.Add(1))
		out := &SendMailOutput{}
		out.Body.Sent = true
		out.Body.Count = cnt
		return out, nil
	})

	// First call with Idempotency-Key
	r1 := httptest.NewRequest(http.MethodPost, "/api/emails/send", strings.NewReader(`{"to":["user@example.com"]}`))
	r1.Header.Set("Content-Type", "application/json")
	r1.Header.Set(HeaderKey, "idem-key-huma-1")
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, r1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want 200: %s", w1.Code, w1.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}

	// Second call with same Idempotency-Key
	r2 := httptest.NewRequest(http.MethodPost, "/api/emails/send", strings.NewReader(`{"to":["user@example.com"]}`))
	r2.Header.Set("Content-Type", "application/json")
	r2.Header.Set(HeaderKey, "idem-key-huma-1")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("second call status = %d, want 200: %s", w2.Code, w2.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1 — second call must be replayed from idempotency store", calls.Load())
	}
	if w2.Header().Get(HeaderReplayed) != "true" {
		t.Errorf("expected %s header = true, got %q", HeaderReplayed, w2.Header().Get(HeaderReplayed))
	}
}
