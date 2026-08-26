package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/octarq-org/octarq/config"
)

type flusherRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flusherRecorder) Flush() {
	f.flushed = true
}

func TestRequestIDContext(t *testing.T) {
	ctx := context.Background()
	if id := RequestID(ctx); id != "" {
		t.Errorf("expected empty string for empty context, got %q", id)
	}

	ctxWithID := context.WithValue(ctx, RequestIDKey, "test-req-123")
	if id := RequestID(ctxWithID); id != "test-req-123" {
		t.Errorf("expected test-req-123, got %q", id)
	}
}

func TestStatusRecorderWriteAndFlush(t *testing.T) {
	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	sr := &statusRecorder{ResponseWriter: rec}

	// Flush should call underlying Flusher
	sr.Flush()
	if !rec.flushed {
		t.Error("expected underlying flusher to be called")
	}

	// Write without WriteHeader sets 200
	n, err := sr.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write error: %v, n=%d", err, n)
	}
	if sr.status != http.StatusOK {
		t.Errorf("expected status 200, got %d", sr.status)
	}

	// Second write preserves status
	_, _ = sr.Write([]byte(" world"))
	if sr.status != http.StatusOK {
		t.Errorf("expected status 200, got %d", sr.status)
	}

	// Non-flusher ResponseWriter Flush() should not panic
	plainRec := httptest.NewRecorder()
	plainSr := &statusRecorder{ResponseWriter: struct{ http.ResponseWriter }{plainRec}}
	plainSr.Flush()
}

func TestRateLimiterSetLimitsAndAllow(t *testing.T) {
	rl := newRateLimiter()
	rl.setLimits(5, 10, 20)

	now := time.Now()
	// Check auth tier
	for i := 0; i < 5; i++ {
		allowed, _ := rl.allow(tierAuth, "1.2.3.4", now)
		if !allowed {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	allowed, retryAfter := rl.allow(tierAuth, "1.2.3.4", now)
	if allowed {
		t.Fatal("request 6 should be denied")
	}
	if retryAfter <= 0 {
		t.Errorf("expected positive retryAfter, got %v", retryAfter)
	}

	// Zero or negative limit disables rate limiting
	rl.setLimits(0, -1, 0)
	allowed, _ = rl.allow(tierAuth, "1.2.3.4", now)
	if !allowed {
		t.Fatal("expected allowed when limit <= 0")
	}
}

func TestMetricsRecordAllStatuses(t *testing.T) {
	m := newMetrics()
	m.record(200)
	m.record(204)
	m.record(301)
	m.record(400)
	m.record(404)
	m.record(500)
	m.record(503)

	snap := m.snapshot()
	if snap["requests_total"].(int64) < 7 {
		t.Errorf("requests_total = %v, expected >= 7", snap["requests_total"])
	}
	if snap["responses_2xx"].(int64) < 2 {
		t.Errorf("responses_2xx = %v, expected >= 2", snap["responses_2xx"])
	}
	if snap["responses_3xx"].(int64) < 1 {
		t.Errorf("responses_3xx = %v, expected >= 1", snap["responses_3xx"])
	}
	if snap["responses_4xx"].(int64) < 2 {
		t.Errorf("responses_4xx = %v, expected >= 2", snap["responses_4xx"])
	}
	if snap["responses_5xx"].(int64) < 2 {
		t.Errorf("responses_5xx = %v, expected >= 2", snap["responses_5xx"])
	}
}

func TestServerNewErrorMissingIndex(t *testing.T) {
	emptyFS := fstest.MapFS{}
	_, err := New(&config.Config{}, nil, mockAPI{}, nil, emptyFS, nil, RuntimeSettings{})
	if err == nil {
		t.Fatal("expected error when webFS lacks index.html")
		return
	}

	validWebFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("index")},
	}
	badMount := []StaticMount{
		{Prefix: "/mount", FS: fstest.MapFS{}},
	}
	_, err = New(&config.Config{}, nil, mockAPI{}, nil, validWebFS, badMount, RuntimeSettings{})
	if err == nil {
		t.Fatal("expected error when mount FS lacks index.html")
		return
	}
}

func TestAssetExistsDirectoryCheck(t *testing.T) {
	fsWithDir := fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte("index")},
		"folder/file.js": &fstest.MapFile{Data: []byte("content")},
	}
	srv, err := New(&config.Config{}, nil, mockAPI{}, nil, fsWithDir, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if srv.assetExists("folder") {
		t.Error("assetExists on a directory should return false")
	}
	if !srv.assetExists("folder/file.js") {
		t.Error("assetExists on a valid file should return true")
	}
	if srv.assetExists("nonexistent.js") {
		t.Error("assetExists on missing file should return false")
	}

	if mountAssetExists(nil, "foo") {
		t.Error("mountAssetExists on nil FS should return false")
	}
	if mountAssetExists(fsWithDir, "folder") {
		t.Error("mountAssetExists on a directory should return false")
	}
	if !mountAssetExists(fsWithDir, "folder/file.js") {
		t.Error("mountAssetExists on a valid file should return true")
	}
}

func TestSanitizeRequestIDAndLoopback(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"req-123_abc", "req-123_abc"},
		{"req/invalid@char", ""},
		{strings.Repeat("a", 150), ""},
	}
	for _, tc := range cases {
		got := sanitizeRequestID(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeRequestID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	reqLoopback4 := &http.Request{RemoteAddr: "127.0.0.1:1234"}
	if !isLoopback(reqLoopback4) {
		t.Error("127.0.0.1:1234 should be loopback")
	}
	reqLoopback6 := &http.Request{RemoteAddr: "[::1]:1234"}
	if !isLoopback(reqLoopback6) {
		t.Error("[::1]:1234 should be loopback")
	}
	reqLocalhost := &http.Request{RemoteAddr: "localhost:1234"}
	if !isLoopback(reqLocalhost) {
		t.Error("localhost:1234 should be loopback")
	}
	reqRemote := &http.Request{RemoteAddr: "192.168.1.1:1234"}
	if isLoopback(reqRemote) {
		t.Error("192.168.1.1:1234 should not be loopback")
	}
	reqEmpty := &http.Request{RemoteAddr: ""}
	if isLoopback(reqEmpty) {
		t.Error("empty remote addr should not be loopback")
	}
}
