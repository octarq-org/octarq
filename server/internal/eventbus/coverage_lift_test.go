package eventbus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestCoverageLift_EventBus_TestSend(t *testing.T) {
	allowSecrets(t)

	// Success response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := TestSend(ctx, 1, 101, ts.URL, "secret_key"); err != nil {
		t.Fatalf("TestSend expected success, got: %v", err)
	}

	// Failure response
	tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsErr.Close()

	if err := TestSend(ctx, 1, 102, tsErr.URL, "secret_key"); err == nil {
		t.Fatalf("TestSend expected error for 500 response")
	}
}

func TestCoverageLift_EventBus_Delivery_BackoffAndSleep(t *testing.T) {
	t.Parallel()

	// backoffFor attempt < 1
	d0 := backoffFor(0)
	if d0 <= 0 {
		t.Fatalf("expected positive backoff for attempt 0, got %v", d0)
	}
	dNeg := backoffFor(-5)
	if dNeg <= 0 {
		t.Fatalf("expected positive backoff for attempt -5, got %v", dNeg)
	}

	// sleepFn with already cancelled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	sleepFn(canceledCtx, 10*time.Minute)

	// truncate
	if got := truncate("abcdef", 3); got != "abc" {
		t.Fatalf("truncate expected abc, got %s", got)
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Fatalf("truncate expected abc, got %s", got)
	}
}

func TestCoverageLift_EventBus_NilDB(t *testing.T) {
	prev := db
	db = nil
	t.Cleanup(func() { db = prev })

	// recordPending
	row := recordPending(delivery{DeliveryID: "del_test", OrgID: 1})
	if row == nil || row.DeliveryID != "del_test" {
		t.Fatalf("expected row to be returned when db is nil")
	}

	// recordResult
	recordResult(&WebhookDelivery{ID: 1}, 1, StatusPending, attemptResult{})

	// Deliveries
	if _, err := Deliveries(context.Background(), 1, "", 10); err == nil {
		t.Fatalf("expected error from Deliveries when db is nil")
	}

	// Replay
	if err := Replay(context.Background(), 1, "del_test"); err == nil {
		t.Fatalf("expected error from Replay when db is nil")
	}

	// Publish
	Publish(1, "test.event", nil)

	// webhookSecret
	if _, err := webhookSecret(context.Background(), 1); err == nil {
		t.Fatalf("expected error from webhookSecret when db is nil")
	}
}

func TestCoverageLift_EventBus_Replay_Edges(t *testing.T) {
	gdb := testDB(t)
	allowSecrets(t)
	ctx := context.Background()

	// Non-existent delivery
	if err := Replay(ctx, 1, "missing-delivery"); err == nil {
		t.Fatalf("expected error for non-existent delivery")
	}

	// Delivery exists, but webhook row is missing
	del := WebhookDelivery{
		DeliveryID: "del-missing-hook",
		OrgID:      1,
		WebhookID:  9999,
		Event:      "test.event",
		URL:        "http://example.com/webhook",
		Body:       `{"hello":"world"}`,
		Status:     StatusPending,
		SignedAt:   time.Now(),
	}
	if err := gdb.Create(&del).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	if err := Replay(ctx, 1, del.DeliveryID); err == nil {
		t.Fatalf("expected error when webhook row is missing")
	}

	// Delivery exists, webhook exists, receiver returns error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	hook := models.Webhook{
		OrgID:   1,
		URL:     ts.URL,
		Secret:  "test-secret",
		Events:  "*",
		Enabled: true,
	}
	if err := gdb.Create(&hook).Error; err != nil {
		t.Fatalf("create hook: %v", err)
	}

	delFailing := WebhookDelivery{
		DeliveryID: "del-failing-replay",
		OrgID:      1,
		WebhookID:  hook.ID,
		Event:      "test.event",
		URL:        ts.URL,
		Body:       `{"hello":"world"}`,
		Status:     StatusPending,
		SignedAt:   time.Now(),
	}
	if err := gdb.Create(&delFailing).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	if err := Replay(ctx, 1, delFailing.DeliveryID); err == nil {
		t.Fatalf("expected error for 502 replay response")
	}

	// Verify delivery status updated to failed
	var reloaded WebhookDelivery
	if err := gdb.First(&reloaded, delFailing.ID).Error; err != nil {
		t.Fatalf("find delivery: %v", err)
	}
	if reloaded.Status != StatusFailed {
		t.Fatalf("expected status failed, got %s", reloaded.Status)
	}
}

func TestCoverageLift_EventBus_Spine_Edges(t *testing.T) {
	t.Parallel()

	// OrgID == 0 or Key == ""
	PublishEnvelope(Envelope{OrgID: 0, Key: "some.key"})
	PublishEnvelope(Envelope{OrgID: 1, Key: ""})

	// spineSend closed
	closedSub := &spineSub{
		closed: true,
		ch:     make(chan Envelope, 1),
	}
	spineSend(closedSub, Envelope{OrgID: 1, Key: "k"})

	// Backpressure slow path: subscriber channel full, env.EntityKey == ""
	subCh, cancel := Subscribe(SubscribeOpts{
		BufferSize: 1,
	})
	defer cancel()

	// Fill buffer
	PublishEnvelope(Envelope{OrgID: 1, Key: "test.key", EntityKey: "", Payload: []byte("first")})
	// Push another without entity key to trigger fallback head drop
	PublishEnvelope(Envelope{OrgID: 1, Key: "test.key", EntityKey: "", Payload: []byte("second")})

	select {
	case <-subCh:
	default:
		t.Fatalf("expected envelope in subCh")
	}
}

type emptyEventReactor struct{}

func (e *emptyEventReactor) Name() string                                 { return "empty" }
func (e *emptyEventReactor) Events() []string                             { return []string{""} }
func (e *emptyEventReactor) MinInterval() time.Duration                   { return 0 }
func (e *emptyEventReactor) React(context.Context, plugin.Envelope) error { return nil }

func TestCoverageLift_EventBus_Reactor_Edges(t *testing.T) {
	t.Parallel()

	// shouldDebounce: eviction sweep when ops%1000 == 0 && len(lastFire) > 10000
	st := &reactorState{
		lastFire: make(map[string]time.Time, 10005),
		ops:      999,
	}
	oldTime := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 10005; i++ {
		st.lastFire["old_"+strconv.Itoa(i)] = oldTime
	}

	// This call makes ops = 1000, triggering sweep
	if st.shouldDebounce("new_item", time.Minute) {
		t.Fatalf("expected new_item not to be debounced")
	}
	if len(st.lastFire) > 50 {
		t.Fatalf("expected stale entries to be evicted, remaining: %d", len(st.lastFire))
	}

	// Start with empty registry
	regEmpty := &ReactorRegistry{}
	stopEmpty := regEmpty.Start(context.Background(), 0)
	stopEmpty()

	// Start with reactor having no non-empty events
	regNoKeys := &ReactorRegistry{}
	regNoKeys.Register(&emptyEventReactor{})
	stopNoKeys := regNoKeys.Start(context.Background(), 2)
	stopNoKeys()
}
