package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/queue"
	"gorm.io/gorm"
)

func newRealtimeStreamTestHandler(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))

	return h.Routes(), db
}

func TestRealtimeStream_AuthRequired(t *testing.T) {
	srv, _ := newRealtimeStreamTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/realtime/stream", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRealtimeStream_InvalidMethod(t *testing.T) {
	srv, _ := newRealtimeStreamTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/realtime/stream", nil)
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", rec.Code)
	}
}

func TestRealtimeStream_RequireOrg(t *testing.T) {
	srv, _ := newRealtimeStreamTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/realtime/stream", nil)
	for _, c := range sessionCookies(t, 1, 0) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for zero org, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRealtimeStream_StreamingSuccessAndIsolation(t *testing.T) {
	srv, _ := newRealtimeStreamTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/realtime/stream", nil).WithContext(ctx)
	for _, c := range sessionCookies(t, 1, 100) {
		req.AddCookie(c)
	}

	buf := newSafeBuffer()
	rec := &safeTestRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		buf:              buf,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.ServeHTTP(rec, req)
	}()

	// Wait for connected event
	if err := buf.waitForString("event: connected", 3*time.Second); err != nil {
		t.Fatalf("wait for connected: %v", err)
	}
	if err := buf.waitForString(`data: {"orgId":100}`, 3*time.Second); err != nil {
		t.Fatalf("wait for orgId 100: %v", err)
	}

	// Publish an event for Org 999 (different tenant). It MUST NOT reach client.
	eventbus.PublishEnvelope(eventbus.Envelope{
		ID:    "other-org-event",
		OrgID: 999,
		Key:   "secret.leak",
	})

	// Publish an event for Org 100 (matching tenant).
	payloadMap := map[string]any{"user": "alice", "action": "invite"}
	rawJSON, _ := json.Marshal(payloadMap)
	eventbus.PublishEnvelope(eventbus.Envelope{
		ID:      "matching-event-1",
		OrgID:   100,
		Key:     "member.invite",
		Payload: rawJSON,
	})

	// Verify client receives matching event
	if err := buf.waitForString("id: matching-event-1", 3*time.Second); err != nil {
		t.Fatalf("wait for matching event id: %v", err)
	}
	if err := buf.waitForString("event: member.invite", 3*time.Second); err != nil {
		t.Fatalf("wait for event member.invite: %v", err)
	}
	if err := buf.waitForString(`"action":"invite"`, 3*time.Second); err != nil {
		t.Fatalf("wait for payload: %v", err)
	}

	// Verify other-org-event never reached
	output := buf.String()
	if strings.Contains(output, "other-org-event") || strings.Contains(output, "secret.leak") {
		t.Fatalf("tenant boundary violation: leaked event in output:\n%s", output)
	}

	// Close context to terminate SSE handler cleanly
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not exit after context cancel")
	}
}

func TestRealtimeStream_EventFilter(t *testing.T) {
	srv, _ := newRealtimeStreamTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/realtime/stream?events=link.click,member.invite", nil).WithContext(ctx)
	for _, c := range sessionCookies(t, 1, 200) {
		req.AddCookie(c)
	}

	buf := newSafeBuffer()
	rec := &safeTestRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		buf:              buf,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.ServeHTTP(rec, req)
	}()

	if err := buf.waitForString("event: connected", 3*time.Second); err != nil {
		t.Fatalf("wait for connected: %v", err)
	}

	// Publish non-subscribed key
	eventbus.PublishEnvelope(eventbus.Envelope{
		ID:    "unfiltered-drop",
		OrgID: 200,
		Key:   "billing.invoice",
	})

	// Publish subscribed key
	eventbus.PublishEnvelope(eventbus.Envelope{
		ID:    "filtered-hit",
		OrgID: 200,
		Key:   "link.click",
	})

	if err := buf.waitForString("id: filtered-hit", 3*time.Second); err != nil {
		t.Fatalf("wait for filtered-hit: %v", err)
	}
	if err := buf.waitForString("event: link.click", 3*time.Second); err != nil {
		t.Fatalf("wait for link.click: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "unfiltered-drop") || strings.Contains(output, "billing.invoice") {
		t.Fatalf("filter violation: dropped event appeared in output:\n%s", output)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not exit after context cancel")
	}
}

func TestRealtimeStream_KeepAlivePing(t *testing.T) {
	oldInterval := realtimePingInterval
	realtimePingInterval = 20 * time.Millisecond
	defer func() { realtimePingInterval = oldInterval }()

	srv, _ := newRealtimeStreamTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/realtime/stream", nil).WithContext(ctx)
	for _, c := range sessionCookies(t, 1, 300) {
		req.AddCookie(c)
	}

	buf := newSafeBuffer()
	rec := &safeTestRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		buf:              buf,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.ServeHTTP(rec, req)
	}()

	if err := buf.waitForString("event: connected", 3*time.Second); err != nil {
		t.Fatalf("wait for connected: %v", err)
	}

	// Should receive ping line
	if err := buf.waitForString(": ping", 3*time.Second); err != nil {
		t.Fatalf("wait for ping: %v", err)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not exit after context cancel")
	}
}

type nonFlusherWriter struct {
	header http.Header
	code   int
}

func newNonFlusherWriter() *nonFlusherWriter {
	return &nonFlusherWriter{header: make(http.Header)}
}

func (w *nonFlusherWriter) Header() http.Header {
	return w.header
}

func (w *nonFlusherWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (w *nonFlusherWriter) WriteHeader(statusCode int) {
	w.code = statusCode
}

func TestRealtimeStream_NonFlusher(t *testing.T) {
	srv, _ := newRealtimeStreamTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/realtime/stream", nil)
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}

	w := newNonFlusherWriter()
	srv.ServeHTTP(w, req)
	if w.code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for non-flusher, got %d", w.code)
	}
}

func TestWriteRealtimeSSE_Errors(t *testing.T) {
	// Invalid JSON payload
	rec := httptest.NewRecorder()
	err := writeRealtimeSSE(rec, nil, "event", "id", make(chan int))
	if err == nil {
		t.Fatal("expected error for unmarshalable data")
	}

	// Failing writer on ID
	fw := &failingHTTPWriter{failOn: 1}
	if err := writeRealtimeSSE(fw, nil, "event", "id", map[string]string{"k": "v"}); err == nil {
		t.Fatal("expected error for failing writer on id")
	}

	// Failing writer on Event
	fw2 := &failingHTTPWriter{failOn: 2}
	if err := writeRealtimeSSE(fw2, nil, "event", "id", map[string]string{"k": "v"}); err == nil {
		t.Fatal("expected error for failing writer on event")
	}

	// Failing writer on Data
	fw3 := &failingHTTPWriter{failOn: 3}
	if err := writeRealtimeSSE(fw3, nil, "event", "id", map[string]string{"k": "v"}); err == nil {
		t.Fatal("expected error for failing writer on data")
	}

	// Failing writer on Final newline
	fw4 := &failingHTTPWriter{failOn: 4}
	if err := writeRealtimeSSE(fw4, nil, "event", "id", map[string]string{"k": "v"}); err == nil {
		t.Fatal("expected error for failing writer on final newline")
	}
}

type failingHTTPWriter struct {
	http.ResponseWriter
	calls  int
	failOn int
}

func (w *failingHTTPWriter) Header() http.Header { return make(http.Header) }
func (w *failingHTTPWriter) WriteHeader(int)     {}
func (w *failingHTTPWriter) Write(b []byte) (int, error) {
	w.calls++
	if w.calls == w.failOn {
		return 0, fmt.Errorf("simulated write error on call %d", w.calls)
	}
	return len(b), nil
}

type safeBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	notify chan struct{}
}

func newSafeBuffer() *safeBuffer {
	return &safeBuffer{
		notify: make(chan struct{}, 100),
	}
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err := b.buf.Write(p)
	select {
	case b.notify <- struct{}{}:
	default:
	}
	return n, err
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *safeBuffer) waitForString(sub string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if strings.Contains(b.String(), sub) {
			return nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.DeadlineExceeded
		}
		select {
		case <-b.notify:
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type safeTestRecorder struct {
	*httptest.ResponseRecorder
	buf *safeBuffer
}

func (r *safeTestRecorder) Write(b []byte) (int, error) {
	return r.buf.Write(b)
}

func (r *safeTestRecorder) Flush() {}
