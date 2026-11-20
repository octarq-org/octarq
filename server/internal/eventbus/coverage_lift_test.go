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

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := TestSend(ctx, 1, 101, ts.URL, 4); err != nil {
		t.Fatalf("TestSend expected success, got: %v", err)
	}

	tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsErr.Close()

	if err := TestSend(ctx, 1, 102, tsErr.URL, 4); err == nil {
		t.Fatalf("TestSend expected error for 500 response")
	}
}

func TestCoverageLift_EventBus_Delivery_BackoffAndSleep(t *testing.T) {
	t.Parallel()

	d0 := backoffFor(0)
	if d0 <= 0 {
		t.Fatalf("expected positive backoff for attempt 0, got %v", d0)
	}
	dNeg := backoffFor(-5)
	if dNeg <= 0 {
		t.Fatalf("expected positive backoff for attempt -5, got %v", dNeg)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	sleepFn(canceledCtx, 10*time.Minute)

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

	row := recordPending(delivery{DeliveryID: "del_test", OrgID: 1})
	if row == nil || row.DeliveryID != "del_test" {
		t.Fatalf("expected row to be returned when db is nil")
	}

	recordResult(&WebhookDelivery{ID: 1}, 1, StatusPending, attemptResult{})

	if _, err := Deliveries(context.Background(), 1, "", 10); err == nil {
		t.Fatalf("expected error from Deliveries when db is nil")
	}

	if err := Replay(context.Background(), 1, "del_test"); err == nil {
		t.Fatalf("expected error from Replay when db is nil")
	}

	Publish(1, "test.event", nil)

	if _, err := webhookVaultItemID(context.Background(), 1, 1); err == nil {
		t.Fatalf("expected error from webhookVaultItemID when db is nil")
	}
}

func TestCoverageLift_EventBus_Replay_Edges(t *testing.T) {
	gdb := testDB(t)
	allowSecrets(t)
	ctx := context.Background()

	if err := Replay(ctx, 1, "missing-delivery"); err == nil {
		t.Fatalf("expected error for non-existent delivery")
	}

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

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	hook := models.Webhook{
		OrgID:       1,
		URL:         ts.URL,
		VaultItemID: 5,
		Events:      "*",
		Enabled:     true,
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

	PublishEnvelope(Envelope{OrgID: 0, Key: "some.key"})
	PublishEnvelope(Envelope{OrgID: 1, Key: ""})

	closedSub := &spineSub{
		closed: true,
		ch:     make(chan Envelope, 1),
	}
	spineSend(closedSub, Envelope{OrgID: 1, Key: "k"})

	subCh, cancel := Subscribe(SubscribeOpts{
		BufferSize: 1,
	})
	defer cancel()

	PublishEnvelope(Envelope{OrgID: 1, Key: "test.key", EntityKey: "", Payload: []byte("first")})
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

	st := &reactorState{
		lastFire: make(map[string]time.Time, 10005),
		ops:      999,
	}
	oldTime := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 10005; i++ {
		st.lastFire["old_"+strconv.Itoa(i)] = oldTime
	}

	if st.shouldDebounce("new_item", time.Minute) {
		t.Fatalf("expected new_item not to be debounced")
	}
	if len(st.lastFire) > 50 {
		t.Fatalf("expected stale entries to be evicted, remaining: %d", len(st.lastFire))
	}

	regEmpty := &ReactorRegistry{}
	stopEmpty := regEmpty.Start(context.Background(), 0)
	stopEmpty()

	regNoKeys := &ReactorRegistry{}
	regNoKeys.Register(&emptyEventReactor{})
	stopNoKeys := regNoKeys.Start(context.Background(), 2)
	stopNoKeys()
}
