package eventbus

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// testDB gives each test its own sqlite file with the webhook + delivery-log
// schema, and restores the package db when it finishes.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "eventbus.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(append(Models(), &models.Webhook{})...); err != nil {
		t.Fatal(err)
	}
	prev := db
	Init(gdb)
	t.Cleanup(func() { Init(prev) })
	return gdb
}

// fastRetries shrinks the retry clock so the loop runs in microseconds, and
// records what backoff each attempt asked for.
func fastRetries(t *testing.T, attempts int) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	var mu sync.Mutex
	prevAttempts, prevSleep := maxAttempts, sleepFn
	maxAttempts = attempts
	sleepFn = func(ctx context.Context, d time.Duration) {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
	}
	t.Cleanup(func() { maxAttempts, sleepFn = prevAttempts, prevSleep })
	return &waits
}

func allowSecrets(t *testing.T) {
	t.Helper()
	SetSecretDecryptor(func(stored string) (string, bool) { return stored, true })
	t.Cleanup(func() { SetSecretDecryptor(nil) })
}

// TestDeliveryRetriesUntilReceiverRecovers is the core reliability guarantee:
// a receiver that 500s twice and then succeeds must still get the event, and
// the delivery log must show it took three attempts.
func TestDeliveryRetriesUntilReceiverRecovers(t *testing.T) {
	gdb := testDB(t)
	allowSecrets(t)
	waits := fastRetries(t, 5)

	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	d := delivery{
		DeliveryID: "dlv-retry",
		OrgID:      7,
		WebhookID:  1,
		Event:      "link.click",
		URL:        ts.URL,
		Secret:     "s",
		Body:       []byte(`{"ok":true}`),
		SignedAt:   time.Now(),
	}
	deliverWithRetry(context.Background(), d)

	if got := hits.Load(); got != 3 {
		t.Fatalf("expected 3 HTTP attempts, got %d", got)
	}
	if len(*waits) != 2 {
		t.Fatalf("expected 2 backoff waits between 3 attempts, got %d", len(*waits))
	}

	var row WebhookDelivery
	if err := gdb.Where("delivery_id = ?", "dlv-retry").First(&row).Error; err != nil {
		t.Fatalf("delivery log row missing: %v", err)
	}
	if row.Status != StatusDelivered {
		t.Errorf("status = %q, want %q", row.Status, StatusDelivered)
	}
	if row.Attempts != 3 {
		t.Errorf("attempts = %d, want 3", row.Attempts)
	}
	if row.ResponseCode != 200 {
		t.Errorf("responseCode = %d, want 200", row.ResponseCode)
	}
	if row.OrgID != 7 {
		t.Errorf("orgID = %d, want 7 (delivery log must stay tenant-scoped)", row.OrgID)
	}
}

// TestDeliveryDeadLettersAfterMaxAttempts pins the bounded attempt count and
// the dead-letter state that makes the failure visible and replayable.
func TestDeliveryDeadLettersAfterMaxAttempts(t *testing.T) {
	gdb := testDB(t)
	allowSecrets(t)
	fastRetries(t, 3)

	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	deliverWithRetry(context.Background(), delivery{
		DeliveryID: "dlv-dead",
		OrgID:      1,
		WebhookID:  1,
		Event:      "link.click",
		URL:        ts.URL,
		Secret:     "s",
		Body:       []byte(`{}`),
		SignedAt:   time.Now(),
	})

	if got := hits.Load(); got != 3 {
		t.Fatalf("expected exactly maxAttempts (3) HTTP attempts, got %d", got)
	}
	var row WebhookDelivery
	if err := gdb.Where("delivery_id = ?", "dlv-dead").First(&row).Error; err != nil {
		t.Fatalf("delivery log row missing: %v", err)
	}
	if row.Status != StatusFailed {
		t.Errorf("status = %q, want %q (dead letter)", row.Status, StatusFailed)
	}
	if row.Attempts != 3 {
		t.Errorf("attempts = %d, want 3", row.Attempts)
	}
	if row.ResponseCode != http.StatusBadGateway {
		t.Errorf("responseCode = %d, want 502", row.ResponseCode)
	}
	if row.LastError == "" {
		t.Error("dead-lettered delivery must record why it failed")
	}
}

// TestPermanentFailureIsNotRetried: a delivery that cannot ever succeed (secret
// will not decrypt) must dead-letter on the first attempt rather than burn the
// whole retry budget.
func TestPermanentFailureIsNotRetried(t *testing.T) {
	gdb := testDB(t)
	SetSecretDecryptor(func(string) (string, bool) { return "", false })
	t.Cleanup(func() { SetSecretDecryptor(nil) })
	waits := fastRetries(t, 5)

	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer ts.Close()

	deliverWithRetry(context.Background(), delivery{
		DeliveryID: "dlv-perm",
		OrgID:      1,
		URL:        ts.URL,
		Secret:     "s",
		Body:       []byte(`{}`),
		SignedAt:   time.Now(),
	})

	if hits.Load() != 0 {
		t.Errorf("unsignable delivery must never hit the network, got %d requests", hits.Load())
	}
	if len(*waits) != 0 {
		t.Errorf("permanent failure must not back off, got %d waits", len(*waits))
	}
	var row WebhookDelivery
	if err := gdb.Where("delivery_id = ?", "dlv-perm").First(&row).Error; err != nil {
		t.Fatalf("delivery log row missing: %v", err)
	}
	if row.Status != StatusFailed || row.Attempts != 1 {
		t.Errorf("got status %q attempts %d, want failed/1", row.Status, row.Attempts)
	}
}

// TestBackoffIsExponentialAndJittered guards the backoff shape itself: the
// window grows per attempt, is capped, and never returns the same value for
// every call (a constant would stampede receivers).
func TestBackoffIsExponentialAndJittered(t *testing.T) {
	prevBase, prevMax := baseBackoff, maxBackoff
	baseBackoff, maxBackoff = 100*time.Millisecond, 2*time.Second
	t.Cleanup(func() { baseBackoff, maxBackoff = prevBase, prevMax })

	// Growth: attempt n's window is [d/2, d] with d = base*2^(n-1).
	for n := 1; n <= 4; n++ {
		want := baseBackoff << (n - 1)
		if want > maxBackoff {
			want = maxBackoff
		}
		for i := 0; i < 50; i++ {
			got := backoffFor(n)
			if got < want/2 || got > want {
				t.Fatalf("backoffFor(%d) = %v, want within [%v, %v]", n, got, want/2, want)
			}
		}
	}

	// Cap: a far-out attempt never exceeds maxBackoff.
	for i := 0; i < 50; i++ {
		if got := backoffFor(30); got > maxBackoff {
			t.Fatalf("backoffFor(30) = %v exceeds cap %v", got, maxBackoff)
		}
	}

	// Jitter: 100 draws of the same attempt must not all be identical.
	seen := map[time.Duration]bool{}
	for i := 0; i < 100; i++ {
		seen[backoffFor(3)] = true
	}
	if len(seen) < 2 {
		t.Error("backoff has no jitter — every retry would fire on the same tick")
	}
}

// TestReplayDetectionHeadersAndSignature is the receiver-side contract: the
// timestamp and delivery id are sent AND signed, so a captured delivery cannot
// be replayed with a fresh timestamp without breaking the signature.
func TestReplayDetectionHeadersAndSignature(t *testing.T) {
	allowSecrets(t)
	secret := "shhh"
	body := []byte(`{"event":"link.click"}`)
	signedAt := time.Unix(1750000000, 0)

	var got http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	d := delivery{
		DeliveryID: "dlv-abc123",
		Event:      "link.click",
		URL:        ts.URL,
		Secret:     secret,
		Body:       body,
		SignedAt:   signedAt,
	}
	if res := attempt(context.Background(), d, 2); res.Err != nil {
		t.Fatalf("attempt: %v", res.Err)
	}

	if got.Get("X-Octarq-Delivery") != "dlv-abc123" {
		t.Errorf("X-Octarq-Delivery = %q, want dlv-abc123", got.Get("X-Octarq-Delivery"))
	}
	if got.Get("X-Octarq-Event") != "link.click" {
		t.Errorf("X-Octarq-Event = %q", got.Get("X-Octarq-Event"))
	}
	if got.Get("X-Octarq-Attempt") != "2" {
		t.Errorf("X-Octarq-Attempt = %q, want 2", got.Get("X-Octarq-Attempt"))
	}
	tsHeader := got.Get("X-Octarq-Timestamp")
	if tsHeader != strconv.FormatInt(signedAt.Unix(), 10) {
		t.Fatalf("X-Octarq-Timestamp = %q, want %d", tsHeader, signedAt.Unix())
	}

	sig := got.Get("X-Octarq-Signature-V2")
	if len(sig) < 4 || sig[:3] != "v2=" {
		t.Fatalf("X-Octarq-Signature-V2 = %q, want v2=… prefix", sig)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsHeader + "." + "dlv-abc123" + "."))
	mac.Write(body)
	if want := hex.EncodeToString(mac.Sum(nil)); sig[3:] != want {
		t.Fatalf("v2 signature = %q, want %q", sig[3:], want)
	}

	// The replay check a receiver performs: the SAME body replayed under a
	// different timestamp must not verify against the v2 signature. If the
	// timestamp were merely a header, this would pass and replay would be
	// undetectable.
	forged := hmac.New(sha256.New, []byte(secret))
	forged.Write(v2Material(signedAt.Add(time.Hour), "dlv-abc123", body))
	if hex.EncodeToString(forged.Sum(nil)) == sig[3:] {
		t.Error("timestamp is not bound into the signature — a captured delivery could be replayed with a fresh timestamp")
	}
}

// TestFanOutIsBounded pins the concurrency ceiling: many hooks subscribed to
// one event must not become one goroutine (and one socket) per hook.
func TestFanOutIsBounded(t *testing.T) {
	gdb := testDB(t)
	allowSecrets(t)
	fastRetries(t, 1)

	var inFlight, peak, served atomic.Int32
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		w.WriteHeader(http.StatusOK)
		served.Add(1)
	}))
	defer ts.Close()

	prevSem := deliverySem
	deliverySem = make(chan struct{}, 2)
	t.Cleanup(func() { deliverySem = prevSem })

	const hooks = 8
	for i := 0; i < hooks; i++ {
		gdb.Create(&models.Webhook{OrgID: 1, Name: "h", URL: ts.URL, Secret: "s", Events: "*", Enabled: true})
	}

	Publish(1, "link.click", map[string]any{"ok": true})

	// Give the fan-out time to saturate the semaphore, then let everyone go.
	deadline := time.Now().Add(3 * time.Second)
	for inFlight.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if inFlight.Load() == 0 {
		t.Fatal("no delivery reached the receiver")
	}
	// Hold the receivers long enough that an unbounded fan-out would show up.
	time.Sleep(100 * time.Millisecond)
	gotPeak := peak.Load()
	close(release)

	if gotPeak > 2 {
		t.Errorf("peak concurrent deliveries = %d, want <= 2 (fan-out is unbounded)", gotPeak)
	}

	// Publish is fire-and-forget, so the delivery goroutines outlive the test
	// body. Wait for all of them before the cleanup swaps the package-level
	// semaphore and DB back — otherwise the teardown races the workers.
	deadline = time.Now().Add(5 * time.Second)
	for served.Load() < hooks && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := served.Load(); got != hooks {
		t.Fatalf("only %d of %d deliveries completed", got, hooks)
	}
	// Every worker has released its slot once the semaphore can be filled.
	for i := 0; i < cap(deliverySem); i++ {
		deliverySem <- struct{}{}
	}
}

// TestReplayResendsOriginalBodyAndDeliveryID: an operator replaying a
// dead-lettered delivery re-sends the same bytes under the same delivery id,
// which is what lets the receiver dedupe instead of double-processing.
func TestReplayResendsOriginalBodyAndDeliveryID(t *testing.T) {
	gdb := testDB(t)
	allowSecrets(t)

	var gotBody, gotID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotID = string(b), r.Header.Get("X-Octarq-Delivery")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	hook := models.Webhook{OrgID: 3, Name: "h", URL: ts.URL, Secret: "s", Events: "*", Enabled: true}
	gdb.Create(&hook)
	gdb.Create(&WebhookDelivery{
		DeliveryID: "dlv-replay", OrgID: 3, WebhookID: hook.ID, Event: "link.click",
		URL: ts.URL, Body: `{"original":true}`, Status: StatusFailed, Attempts: 5,
	})

	if err := Replay(context.Background(), 3, "dlv-replay"); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if gotBody != `{"original":true}` {
		t.Errorf("replayed body = %q, want the original bytes", gotBody)
	}
	if gotID != "dlv-replay" {
		t.Errorf("replayed delivery id = %q, want dlv-replay (a replay is not a new event)", gotID)
	}

	var row WebhookDelivery
	gdb.Where("delivery_id = ?", "dlv-replay").First(&row)
	if row.Status != StatusDelivered || row.Attempts != 6 {
		t.Errorf("after replay: status %q attempts %d, want delivered/6", row.Status, row.Attempts)
	}

	// Another org must not be able to replay it.
	if err := Replay(context.Background(), 4, "dlv-replay"); err == nil {
		t.Error("cross-tenant replay must fail")
	}
}

func TestDeliveriesListing(t *testing.T) {
	gdb := testDB(t)
	for i := 0; i < 3; i++ {
		gdb.Create(&WebhookDelivery{DeliveryID: "d" + strconv.Itoa(i), OrgID: 1, WebhookID: 1, Event: "e", URL: "http://x", Status: StatusFailed})
	}
	gdb.Create(&WebhookDelivery{DeliveryID: "other", OrgID: 2, WebhookID: 1, Event: "e", URL: "http://x", Status: StatusDelivered})

	rows, err := Deliveries(context.Background(), 1, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (org scoping)", len(rows))
	}
	if rows[0].DeliveryID != "d2" {
		t.Errorf("expected newest first, got %q", rows[0].DeliveryID)
	}
	failed, err := Deliveries(context.Background(), 1, StatusFailed, 10)
	if err != nil || len(failed) != 3 {
		t.Errorf("status filter: %d rows, %v", len(failed), err)
	}
	none, _ := Deliveries(context.Background(), 1, StatusDelivered, 10)
	if len(none) != 0 {
		t.Errorf("expected no delivered rows for org 1, got %d", len(none))
	}
}
