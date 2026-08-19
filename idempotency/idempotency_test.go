package idempotency

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newStore(t *testing.T, opts ...Option) (*Store, *gorm.DB) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "idem.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(Models()...); err != nil {
		t.Fatal(err)
	}
	return New(gdb, opts...), gdb
}

// countingHandler answers 201 with a body that changes every call, so a
// replayed response is distinguishable from a re-executed one.
func countingHandler(calls *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"id":%d}`, n)
	})
}

func org7(*http.Request) uint { return 7 }

func post(t *testing.T, h http.Handler, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/emails/send", strings.NewReader(body))
	if key != "" {
		r.Header.Set(HeaderKey, key)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// TestReplayReturnsFirstResponseWithoutRunningHandler is the whole point: the
// second POST with the same key must not send a second email.
func TestReplayReturnsFirstResponseWithoutRunningHandler(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))

	first := post(t, h, "key-1", `{"to":"a@example.com"}`)
	second := post(t, h, "key-1", `{"to":"a@example.com"}`)

	if got := calls.Load(); got != 1 {
		t.Fatalf("handler ran %d times, want 1 — the retry was executed again", got)
	}
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("status codes: first %d second %d, want both 201", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("replay body = %q, want the stored %q", second.Body.String(), first.Body.String())
	}
	if second.Header().Get(HeaderReplayed) != "true" {
		t.Error("replayed response must be labelled with Idempotency-Replayed")
	}
	if first.Header().Get(HeaderReplayed) != "" {
		t.Error("the original response must not claim to be a replay")
	}
	if ct := second.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("replay Content-Type = %q, want the stored application/json", ct)
	}
}

// TestNoKeyMeansNoDeduplication: the mechanism is opt-in per request.
func TestNoKeyMeansNoDeduplication(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))

	post(t, h, "", `{"a":1}`)
	post(t, h, "", `{"a":1}`)
	if calls.Load() != 2 {
		t.Errorf("handler ran %d times, want 2 — requests without a key must not be de-duplicated", calls.Load())
	}
}

// TestSafeMethodsBypass: GET is already idempotent and must not be captured.
func TestSafeMethodsBypass(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/links", nil)
		r.Header.Set(HeaderKey, "key-get")
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	if calls.Load() != 2 {
		t.Errorf("GET ran %d times, want 2", calls.Load())
	}
}

// TestKeyScopedPerOrgAndEndpoint: the same key string from another workspace,
// or against another endpoint, is a different operation.
func TestKeyScopedPerOrgAndEndpoint(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32
	inner := countingHandler(&calls)

	var org uint = 7
	h := s.Middleware(func(*http.Request) uint { return org })(inner)

	post(t, h, "shared", `{"x":1}`)
	org = 8
	post(t, h, "shared", `{"x":1}`)
	if calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2 — one tenant's key must not answer another's request", calls.Load())
	}

	org = 7
	r := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(`{"x":1}`))
	r.Header.Set(HeaderKey, "shared")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if calls.Load() != 3 {
		t.Errorf("handler ran %d times, want 3 — the same key on a different endpoint is a different operation", calls.Load())
	}
}

// TestKeyReuseWithDifferentBodyIsRejected: replaying an unrelated stored
// response would be worse than an error.
func TestKeyReuseWithDifferentBodyIsRejected(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))

	post(t, h, "key-2", `{"to":"a@example.com"}`)
	w := post(t, h, "key-2", `{"to":"victim@example.com"}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 for a reused key with a different body", w.Code)
	}
	if calls.Load() != 1 {
		t.Errorf("handler ran %d times, want 1", calls.Load())
	}
}

// TestConcurrentDuplicateGetsConflictNotDoubleExecution covers the real race:
// the client's retry arrives while the original is still running.
func TestConcurrentDuplicateGetsConflictNotDoubleExecution(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		w.WriteHeader(http.StatusCreated)
	})
	h := s.Middleware(org7)(slow)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		post(t, h, "key-race", `{"a":1}`)
	}()
	<-entered

	second := post(t, h, "key-race", `{"a":1}`)
	if second.Code != http.StatusConflict {
		t.Errorf("in-flight duplicate status = %d, want 409", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("409 for an in-flight duplicate should tell the client when to retry")
	}
	close(release)
	wg.Wait()

	if calls.Load() != 1 {
		t.Errorf("handler ran %d times, want 1", calls.Load())
	}
}

// TestServerErrorReleasesTheKey: a 500 must not be cached, or the client's
// retry (the entire reason for the header) is answered with the failure.
func TestServerErrorReleasesTheKey(t *testing.T) {
	s, gdb := newStore(t)
	var calls atomic.Int32
	h := s.Middleware(org7)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))

	if w := post(t, h, "key-500", `{"a":1}`); w.Code != http.StatusInternalServerError {
		t.Fatalf("first status = %d, want 500", w.Code)
	}
	var n int64
	gdb.Model(&Record{}).Count(&n)
	if n != 0 {
		t.Errorf("%d records left after a 5xx, want 0 (the key must be released)", n)
	}
	if w := post(t, h, "key-500", `{"a":1}`); w.Code != http.StatusCreated {
		t.Errorf("retry after a 5xx status = %d, want 201", w.Code)
	}
	if calls.Load() != 2 {
		t.Errorf("handler ran %d times, want 2", calls.Load())
	}
}

// TestPanicReleasesTheKey: a handler panic must not leave the key stuck
// "in progress" forever.
func TestPanicReleasesTheKey(t *testing.T) {
	s, gdb := newStore(t)
	h := s.Middleware(org7)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	func() {
		defer func() { _ = recover() }()
		post(t, h, "key-panic", `{"a":1}`)
	}()

	var n int64
	gdb.Model(&Record{}).Count(&n)
	if n != 0 {
		t.Errorf("%d records left after a panic, want 0", n)
	}
}

// TestExpiredKeyIsReclaimed: past the TTL the same key is a new operation.
func TestExpiredKeyIsReclaimed(t *testing.T) {
	now := time.Now()
	s, _ := newStore(t, WithTTL(time.Minute), WithClock(func() time.Time { return now }))
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))

	post(t, h, "key-ttl", `{"a":1}`)
	post(t, h, "key-ttl", `{"a":1}`)
	if calls.Load() != 1 {
		t.Fatalf("within TTL the handler ran %d times, want 1", calls.Load())
	}

	now = now.Add(2 * time.Minute)
	w := post(t, h, "key-ttl", `{"a":1}`)
	if calls.Load() != 2 {
		t.Errorf("after the TTL the handler ran %d times, want 2", calls.Load())
	}
	if w.Header().Get(HeaderReplayed) != "" {
		t.Error("an expired record must not be replayed")
	}
}

func TestPurgeDropsExpiredOnly(t *testing.T) {
	now := time.Now()
	s, gdb := newStore(t, WithClock(func() time.Time { return now }))
	gdb.Create(&Record{OrgID: 1, Endpoint: "POST /a", Key: "old", RequestHash: "h", State: stateDone, ExpiresAt: now.Add(-time.Hour)})
	gdb.Create(&Record{OrgID: 1, Endpoint: "POST /a", Key: "new", RequestHash: "h", State: stateDone, ExpiresAt: now.Add(time.Hour)})

	n, err := s.Purge()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("purged %d rows, want 1", n)
	}
	var left int64
	gdb.Model(&Record{}).Count(&left)
	if left != 1 {
		t.Errorf("%d rows left, want 1", left)
	}
}

// TestUnscopedRequestBypasses: with no workspace resolved there is no tenant
// namespace to file the key under, so it must not be stored globally.
func TestUnscopedRequestBypasses(t *testing.T) {
	s, gdb := newStore(t)
	var calls atomic.Int32
	h := s.Middleware(func(*http.Request) uint { return 0 })(countingHandler(&calls))

	post(t, h, "key-anon", `{"a":1}`)
	post(t, h, "key-anon", `{"a":1}`)
	if calls.Load() != 2 {
		t.Errorf("handler ran %d times, want 2", calls.Load())
	}
	var n int64
	gdb.Model(&Record{}).Count(&n)
	if n != 0 {
		t.Errorf("%d records stored for an unscoped request, want 0", n)
	}
}

func TestOversizedKeyRejected(t *testing.T) {
	s, _ := newStore(t)
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))
	if w := post(t, h, strings.Repeat("k", 256), `{"a":1}`); w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if calls.Load() != 0 {
		t.Error("handler must not run for a rejected key")
	}
}

// TestHandlerStillSeesTheBody guards the body-buffering: reading the request
// for hashing must not consume it out from under the handler.
func TestHandlerStillSeesTheBody(t *testing.T) {
	s, _ := newStore(t)
	var got string
	h := s.Middleware(org7)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		got = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	post(t, h, "key-body", `{"to":"a@example.com"}`)
	if got != `{"to":"a@example.com"}` {
		t.Errorf("handler saw body %q, want the original", got)
	}
}

// TestOversizedResponseIsNotStored: replaying a truncated body would be worse
// than not replaying, so a response past the ceiling is served but not kept.
func TestOversizedResponseIsNotStored(t *testing.T) {
	s, _ := newStore(t, WithMaxBody(16))
	var calls atomic.Int32
	h := s.Middleware(org7)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(strings.Repeat("x", 64)))
	}))

	first := post(t, h, "key-big", `{"a":1}`)
	if len(first.Body.String()) != 64 {
		t.Fatalf("client got %d bytes, want the full 64 — capture must not truncate the real response", len(first.Body.String()))
	}
	second := post(t, h, "key-big", `{"a":1}`)
	if calls.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1 — an uncapturable response must still not be re-executed", calls.Load())
	}
	if second.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 for a completed request whose response was not stored", second.Code)
	}
	if second.Header().Get("Idempotency-Original-Status") != "201" {
		t.Errorf("Idempotency-Original-Status = %q, want 201", second.Header().Get("Idempotency-Original-Status"))
	}
	if second.Body.Len() == 0 {
		t.Error("an unreplayable repeat must explain itself, not return an empty body")
	}
}

// TestOversizedRequestRejected: a request too large to hash cannot be
// de-duplicated, and answering it anyway would be a silent false promise.
func TestOversizedRequestRejected(t *testing.T) {
	s, _ := newStore(t, WithMaxBody(8))
	var calls atomic.Int32
	h := s.Middleware(org7)(countingHandler(&calls))
	if w := post(t, h, "key-huge", strings.Repeat("y", 64)); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", w.Code)
	}
	if calls.Load() != 0 {
		t.Error("handler ran for a request that could not be de-duplicated")
	}
}

// TestFlushingHandlerStillWorks: a streaming handler must keep streaming; its
// response is simply not stored for replay.
func TestFlushingHandlerStillWorks(t *testing.T) {
	s, _ := newStore(t)
	h := s.Middleware(org7)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk"))
		w.(http.Flusher).Flush()
	}))
	w := post(t, h, "key-stream", `{"a":1}`)
	if w.Body.String() != "chunk" {
		t.Errorf("body = %q, want chunk", w.Body.String())
	}
	replayed := post(t, h, "key-stream", `{"a":1}`)
	if replayed.Header().Get(HeaderReplayed) == "true" {
		t.Error("a streamed response was never captured, so it must not be presented as a replay")
	}
	if replayed.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", replayed.Code)
	}
}

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
