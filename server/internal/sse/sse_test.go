package sse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testAuth(org uint64) AuthFunc {
	return func(r *http.Request) (uint64, error) {
		if org == 0 {
			return 0, errors.New("anonymous")
		}
		return org, nil
	}
}

// readStream consumes SSE frames until ctx ends, collecting data chunks delimited by double newline.
func readStream(t *testing.T, ctx context.Context, body io.Reader, out chan<- string) {
	t.Helper()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 512)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				idx := indexDoubleNewline(buf)
				if idx < 0 {
					break
				}
				out <- string(buf[:idx])
				buf = buf[idx+2:]
			}
		}
		if err != nil {
			return
		}
	}
}

func indexDoubleNewline(b []byte) int {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '\n' && b[i+1] == '\n' {
			return i
		}
	}
	return -1
}

func TestRegisterUnregister(t *testing.T) {
	h := New()
	if _, err := h.Register(0); !errors.Is(err, ErrZeroOrg) {
		t.Fatalf("error = %v, want ErrZeroOrg", err)
	}
	a, err := h.Register(1)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	b, err := h.Register(1)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if h.Connections(1) != 2 || a.OrgID() != 1 {
		t.Fatalf("connections = %d", h.Connections(1))
	}
	h.Unregister(a)
	h.Unregister(a) // idempotent
	if h.Connections(1) != 1 {
		t.Fatalf("connections = %d, want 1", h.Connections(1))
	}
	h.Unregister(b)
	if h.Connections(1) != 0 {
		t.Fatalf("connections = %d, want 0", h.Connections(1))
	}
	h.Unregister(nil) // no panic
}

// TestNonBlockingDrops satisfies:
// Saturate a slow subscriber channel and verify dropped.Load() > 0 without blocking the publisher.
func TestNonBlockingDrops(t *testing.T) {
	h := New(WithBuffer(2))
	c, err := h.Register(1)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer h.Unregister(c)

	// Publish more events than buffer capacity (2) without reading from subscriber channel.
	start := time.Now()
	for i := 0; i < 10; i++ {
		if err := h.Broadcast(1, fmt.Sprintf("evt-%d", i)); err != nil {
			t.Fatalf("Broadcast failed: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("publisher blocked: took %v", elapsed)
	}

	if c.dropped.Load() <= 0 {
		t.Fatalf("expected conn dropped.Load() > 0, got %d", c.dropped.Load())
	}
	if c.Dropped() <= 0 {
		t.Fatalf("expected conn Dropped() > 0, got %d", c.Dropped())
	}
	if h.dropped.Load() <= 0 {
		t.Fatalf("expected hub dropped.Load() > 0, got %d", h.dropped.Load())
	}
	if h.Dropped() <= 0 {
		t.Fatalf("expected hub Dropped() > 0, got %d", h.Dropped())
	}
}

// TestPerTenantConnectionLimit satisfies:
// Register 32 connections for one tenant, verify 33rd returns ErrTenantFull.
func TestPerTenantConnectionLimit(t *testing.T) {
	h := New() // DefaultTenantCap = 32
	conns := make([]*Conn, 0, DefaultTenantCap)
	for i := 0; i < DefaultTenantCap; i++ {
		c, err := h.Register(1)
		if err != nil {
			t.Fatalf("Register connection %d: %v", i+1, err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			h.Unregister(c)
		}
	}()

	// 33rd connection must fail with ErrTenantFull
	_, err := h.Register(1)
	if !errors.Is(err, ErrTenantFull) {
		t.Fatalf("33rd connection error = %v, want ErrTenantFull", err)
	}
}

// TestCrossTenantIsolation satisfies:
// Broadcast to Org 1 does not reach Org 2.
func TestCrossTenantIsolation(t *testing.T) {
	h := New()
	c1, err := h.Register(1)
	if err != nil {
		t.Fatalf("Register org 1: %v", err)
	}
	defer h.Unregister(c1)

	c2, err := h.Register(2)
	if err != nil {
		t.Fatalf("Register org 2: %v", err)
	}
	defer h.Unregister(c2)

	if err := h.Broadcast(1, "evt-tenant1"); err != nil {
		t.Fatalf("Broadcast to org 1: %v", err)
	}

	select {
	case id := <-c1.queue:
		if id != "evt-tenant1" {
			t.Fatalf("got %q, want evt-tenant1", id)
		}
	case <-time.After(time.Second):
		t.Fatal("org 1 did not receive its event")
	}

	select {
	case id := <-c2.queue:
		t.Fatalf("cross-tenant leak: org 2 received event %q", id)
	case <-time.After(100 * time.Millisecond):
		// Expected: nothing received by org 2
	}
}

// TestHeartbeatEmitsPingComments satisfies:
// Test heartbeat emitting ping comments.
func TestHeartbeatEmitsPingComments(t *testing.T) {
	h := New(WithAuth(testAuth(1)), WithHeartbeat(25*time.Millisecond))
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	frames := make(chan string, 16)
	go readStream(t, ctx, resp.Body, frames)

	timeout := time.After(2 * time.Second)
	var gotPingComment bool
	for !gotPingComment {
		select {
		case f := <-frames:
			if strings.Contains(f, ": ping") {
				gotPingComment = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for heartbeat ping comment")
		}
	}
}

func TestBroadcastIsTenantIsolated(t *testing.T) {
	h := New()
	a, _ := h.Register(1)
	b, _ := h.Register(2)
	defer h.Unregister(a)
	defer h.Unregister(b)

	if err := h.Broadcast(1, "evt-1"); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	select {
	case id := <-a.queue:
		if id != "evt-1" {
			t.Fatalf("id = %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("tenant 1 did not receive its broadcast")
	}
	select {
	case id := <-b.queue:
		t.Fatalf("tenant 2 received tenant 1's event %q", id)
	case <-time.After(50 * time.Millisecond):
	}

	if err := h.Broadcast(0, "x"); !errors.Is(err, ErrZeroOrg) {
		t.Fatalf("error = %v, want ErrZeroOrg", err)
	}
	if err := h.Broadcast(1, ""); err == nil {
		t.Fatal("empty event id accepted")
	}
	if err := h.Broadcast(1, strings.Repeat("x", DefaultMaxEventID+1)); !errors.Is(err, ErrEventIDTooBig) {
		t.Fatalf("error = %v, want ErrEventIDTooBig", err)
	}
}

func TestBroadcastRejectsControlCharacters(t *testing.T) {
	h := New()
	for _, id := range []string{
		"run:abc\nid: order:evil",
		"run:a\rb",
		"run:a\x00b",
	} {
		if err := h.Broadcast(1, id); !errors.Is(err, ErrEventIDInvalid) {
			t.Fatalf("Broadcast(%q) error = %v, want ErrEventIDInvalid", id, err)
		}
	}
}

func TestIDFrameFlattensNewlines(t *testing.T) {
	got := IDFrame("run:a\nid: order:evil")
	body := strings.TrimSuffix(got, "\n\n")
	if strings.Contains(body, "\n") {
		t.Fatalf("IDFrame body spans multiple lines: %q", got)
	}
	if !strings.Contains(body, "run:a") || !strings.Contains(body, "order:evil") {
		t.Fatalf("IDFrame lost identifier text: %q", got)
	}
}

func TestStreamCarriesIdentifiersOnly(t *testing.T) {
	h := New(WithAuth(testAuth(1)), WithHeartbeat(50*time.Millisecond))
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}

	frames := make(chan string, 16)
	go readStream(t, ctx, resp.Body, frames)

	deadline := time.Now().Add(2 * time.Second)
	for h.Connections(1) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if err := h.Broadcast(1, "approval-42-decided"); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	var gotID bool
	var gotPing bool
	timeout := time.After(2 * time.Second)
	for !gotID || !gotPing {
		select {
		case f := <-frames:
			if strings.Contains(f, "id: approval-42-decided") {
				gotID = true
			}
			if strings.Contains(f, ": ping") {
				gotPing = true
			}
			for _, line := range strings.Split(f, "\n") {
				if strings.HasPrefix(line, "data:") {
					t.Fatalf("event body leaked onto the wire: %q", f)
				}
			}
		case <-timeout:
			t.Fatalf("gotID=%v gotPing=%v", gotID, gotPing)
		}
	}
	cancel()
}

func TestUnauthenticatedAndCSRFRejected(t *testing.T) {
	h := New(WithOrigins(".example.com"))

	// No authenticator at all.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	// Authenticator that rejects.
	h2 := New(WithAuth(testAuth(0)))
	rec = httptest.NewRecorder()
	h2.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	// Authenticated with CSRF hook failure.
	csrfErr := errors.New("csrf mismatch")
	hCSRF := New(WithAuth(testAuth(1)), WithCSRFVerify(func(r *http.Request, orgID uint64) error {
		return csrfErr
	}))
	rec = httptest.NewRecorder()
	hCSRF.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 on CSRF failure", rec.Code)
	}

	// Authenticated but cross-site origin.
	h3 := New(WithAuth(testAuth(1)), WithOrigins(".example.com"))
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set("Origin", "https://evil.test")
	rec = httptest.NewRecorder()
	h3.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}

	// Allowlisted origin passes the gate.
	req = httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set("Origin", "https://app.example.com")
	ctx, cancel := context.WithCancel(req.Context())
	rec = httptest.NewRecorder()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	h3.ServeHTTP(rec, req.WithContext(ctx))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestOriginSuffixMustBeUnambiguous(t *testing.T) {
	preflight := func(h *Hub, origin string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
		req.Header.Set("Origin", origin)
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		rec := httptest.NewRecorder()
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		h.ServeHTTP(rec, req.WithContext(ctx))
		return rec.Code
	}

	bare := New(WithAuth(testAuth(1)), WithOrigins("example.com"))
	if code := preflight(bare, "https://notexample.com"); code != http.StatusForbidden {
		t.Fatalf("bare suffix accepted an unrelated domain: status %d", code)
	}
	if code := preflight(bare, "https://app.example.com"); code != http.StatusForbidden {
		t.Fatalf("ambiguous entry matched anything: status %d", code)
	}

	family := New(WithAuth(testAuth(1)), WithOrigins(".example.com"))
	if code := preflight(family, "https://notexample.com"); code != http.StatusForbidden {
		t.Fatalf("family suffix accepted an unrelated domain: status %d", code)
	}
	if code := preflight(family, "https://app.example.com"); code != http.StatusOK {
		t.Fatalf("family suffix refused its own domain: status %d", code)
	}
}

func TestTenantCapAndBackpressure(t *testing.T) {
	h := New(WithAuth(testAuth(1)), WithTenantCap(2), WithBuffer(1))
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	r1, _ := http.NewRequestWithContext(ctx1, http.MethodGet, srv.URL, nil)
	resp1, err := http.DefaultClient.Do(r1)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp1.Body.Close()

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	r2, _ := http.NewRequestWithContext(ctx2, http.MethodGet, srv.URL, nil)
	resp2, err := http.DefaultClient.Do(r2)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp2.Body.Close()

	deadline := time.Now().Add(2 * time.Second)
	for h.Connections(1) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if h.Connections(1) != 2 {
		t.Fatalf("connections = %d, want 2", h.Connections(1))
	}

	// Third connection is refused with 429.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}

	start := time.Now()
	const n = 50
	for i := 0; i < n; i++ {
		if err := h.Broadcast(1, fmt.Sprintf("evt-%d", i)); err != nil {
			t.Fatalf("Broadcast %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("broadcast loop stalled: %v for %d events", elapsed, n)
	}
	cancel1()
	cancel2()
}

func TestSlowConsumerDoesNotStallPeer(t *testing.T) {
	h := New(WithBuffer(1))
	slow, _ := h.Register(1)
	fast, _ := h.Register(1)
	defer h.Unregister(slow)
	defer h.Unregister(fast)

	const n = 20
	for i := 0; i < n; i++ {
		if err := h.Broadcast(1, fmt.Sprintf("evt-%d", i)); err != nil {
			t.Fatalf("Broadcast: %v", err)
		}
	}

	got := 0
	for {
		select {
		case <-fast.queue:
			got++
		default:
			goto done
		}
	}
done:
	if got == 0 {
		t.Fatal("fast peer received nothing")
	}
	if slow.Dropped() == 0 {
		t.Logf("note: slow peer dropped nothing; fast got %d", got)
	}
}

func TestDisconnectLeavesNoGoroutine(t *testing.T) {
	h := New(WithAuth(testAuth(1)), WithHeartbeat(10*time.Millisecond))
	srv := httptest.NewServer(h)
	defer srv.Close()

	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	const streams = 5
	var wg sync.WaitGroup
	for i := 0; i < streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(3 * time.Second)
	for h.Connections(1) != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.Connections(1) != 0 {
		t.Fatalf("connections = %d after disconnect", h.Connections(1))
	}

	deadline = time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before+2 {
		t.Fatalf("goroutines: before=%d after=%d — stream handlers leaked", before, got)
	}
}

func TestDrainEndsStreams(t *testing.T) {
	h := New()
	var total atomic.Int64
	var wg sync.WaitGroup
	for org := uint64(1); org <= 3; org++ {
		for i := 0; i < 3; i++ {
			c, err := h.Register(org)
			if err != nil {
				t.Fatalf("Register: %v", err)
			}
			wg.Add(1)
			go func(c *Conn) {
				defer wg.Done()
				<-c.Done()
				total.Add(1)
			}(c)
		}
	}
	h.Drain(context.Background())
	wg.Wait()
	if total.Load() != 9 {
		t.Fatalf("drained %d of 9 connections", total.Load())
	}
	if h.Connections(1)+h.Connections(2)+h.Connections(3) != 0 {
		t.Fatal("connections remain after Drain")
	}
}

func TestHubClose(t *testing.T) {
	h := New()
	c, err := h.Register(1)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	closed := make(chan struct{})
	go func() {
		<-c.Done()
		close(closed)
	}()
	h.Close()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("connection not closed by Hub.Close")
	}
	if h.Connections(1) != 0 {
		t.Fatalf("connections = %d, want 0", h.Connections(1))
	}
}

func TestConnClose(t *testing.T) {
	c := &Conn{
		orgID: 1,
		queue: make(chan string, 1),
		done:  make(chan struct{}),
	}
	c.Close()
	select {
	case <-c.Done():
	default:
		t.Fatal("c.Done() not closed after c.Close()")
	}
	// idempotent
	c.Close()
	if !c.closed.Load() {
		t.Fatal("c.closed is not true")
	}
	if q := c.Queue(); q == nil {
		t.Fatal("c.Queue() returned nil")
	}
}

func TestSubscribe(t *testing.T) {
	h := New(WithAuth(testAuth(1)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Subscribe(w, r)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	cancel()
}

func TestPrepareNonFlusher(t *testing.T) {
	type nonFlusher struct {
		http.ResponseWriter
	}
	rec := httptest.NewRecorder()
	nf := nonFlusher{ResponseWriter: rec}
	_, ok := Prepare(nf)
	if ok {
		t.Fatal("expected Prepare to fail on non-flusher")
	}
}

func TestOptionsAreFailClosed(t *testing.T) {
	h := New(WithHeartbeat(0), WithBuffer(0), WithTenantCap(0), WithMaxEventID(0), nil)
	if h.heartbeat != DefaultHeartbeat || h.buffer != DefaultBuffer || h.tenantCap != DefaultTenantCap || h.maxIDLen != DefaultMaxEventID {
		t.Fatal("zero options overrode defaults")
	}
}

func TestHubDroppedAggregates(t *testing.T) {
	h := New(WithBuffer(1))
	c, err := h.Register(1)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := h.Broadcast(1, fmt.Sprintf("evt-%d", i)); err != nil {
			t.Fatalf("Broadcast: %v", err)
		}
	}
	// buffer 1: one queued, two overflow.
	if got := h.Dropped(); got != 2 {
		t.Fatalf("hub dropped = %d, want 2", got)
	}
	if got := c.Dropped(); got != 2 {
		t.Fatalf("conn dropped = %d, want 2", got)
	}
	if err := h.Broadcast(2, "evt-none"); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if got := h.Dropped(); got != 2 {
		t.Fatalf("hub dropped after empty broadcast = %d, want 2", got)
	}
	h.Unregister(c)
	if got := h.Dropped(); got != 2 {
		t.Fatalf("hub dropped after unregister = %d, want 2", got)
	}
}
