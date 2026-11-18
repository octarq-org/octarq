package eventbus

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func TestIsSubscribed(t *testing.T) {
	cases := []struct {
		subs   string
		event  string
		expect bool
	}{
		{"*", "link.click", true},
		{"link.click", "link.click", true},
		{"link.click,email.receive", "email.receive", true},
		{"link.click, email.receive ", "email.receive", true},
		{"link.click", "email.receive", false},
		{"", "link.click", false},
	}

	for _, c := range cases {
		if got := isSubscribed(c.subs, c.event); got != c.expect {
			t.Errorf("isSubscribed(%q, %q) = %v, want %v", c.subs, c.event, got, c.expect)
		}
	}
}

func TestDeliverAndHMACSignature(t *testing.T) {
	allowSecrets(t)

	secret := "test-webhook-secret-key"
	payload := []byte(`{"event":"test.event","timestamp":"2026-06-29T10:00:00Z","orgId":1,"data":{"hello":"world"}}`)

	var receivedHeaders http.Header
	var receivedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	signedAt := time.Unix(1750000000, 0)
	d := delivery{
		DeliveryID:  "dlv-fixed-1",
		OrgID:       1,
		Event:       "test.event",
		URL:         ts.URL,
		VaultItemID: 3,
		Body:        payload,
		SignedAt:    signedAt,
	}
	if res := attempt(ctx, d, 1); res.Err != nil {
		t.Fatalf("attempt failed: %v", res.Err)
	}

	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", receivedHeaders.Get("Content-Type"))
	}

	sigHeader := receivedHeaders.Get("X-Octarq-Signature")
	if sigHeader == "" {
		t.Fatal("missing X-Octarq-Signature header")
	}

	if !strings.HasPrefix(sigHeader, "sha256=") {
		t.Fatalf("invalid signature format: %q", sigHeader)
	}

	gotSig := sigHeader[len("sha256="):]

	// v1 signature: HMAC over the body alone (unchanged, for existing receivers).
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if gotSig != expectedSig {
		t.Errorf("signature mismatch: got %q, want %q", gotSig, expectedSig)
	}

	if string(receivedBody) != string(payload) {
		t.Errorf("body mismatch: got %q, want %q", string(receivedBody), string(payload))
	}
}

func TestDeliverErrors(t *testing.T) {
	run := func(url string, vaultItemID uint) attemptResult {
		return attempt(context.Background(), delivery{
			DeliveryID:  "dlv-err",
			OrgID:       1,
			URL:         url,
			VaultItemID: vaultItemID,
			Body:        []byte("{}"),
			SignedAt:    time.Now(),
		}, 1)
	}

	// 1. Invalid URL scheme — permanent, never retried.
	if res := run("ftp://example.com/hook", 1); !errors.Is(res.Err, errPermanent) {
		t.Errorf("bad scheme should be permanent, got %v", res.Err)
	}

	// 2. Vault service lookup failure — permanent.
	SetVaultService(&testVaultService{err: errors.New("vault error")})
	if res := run("http://example.com/hook", 1); !errors.Is(res.Err, errPermanent) {
		t.Errorf("unresolvable secret should be permanent, got %v", res.Err)
	}

	// 3. No vault service registered — permanent.
	SetVaultService(nil)
	if res := run("http://example.com/hook", 1); !errors.Is(res.Err, errPermanent) {
		t.Errorf("missing vault service should be permanent, got %v", res.Err)
	}

	// 4. Server returns HTTP 500 — retryable, and the status is reported.
	allowSecrets(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	res := run(ts.URL, 1)
	if res.Err == nil || errors.Is(res.Err, errPermanent) {
		t.Errorf("HTTP 500 should be a retryable error, got %v", res.Err)
	}
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected recorded status 500, got %d", res.StatusCode)
	}
}

func TestSigningSecret(t *testing.T) {
	ctx := context.Background()
	// 1. Zero VaultItemID
	if _, err := signingSecret(ctx, 1, 0); err == nil {
		t.Errorf("expected error for zero vaultItemID")
	}

	// 2. Nil vault service
	SetVaultService(nil)
	_, err := signingSecret(ctx, 1, 1)
	if err == nil || !strings.Contains(err.Error(), "no vault service registered") {
		t.Errorf("expected no vault service error, got %v", err)
	}

	// 3. Vault service failure
	SetVaultService(&testVaultService{err: errors.New("boom")})
	_, err = signingSecret(ctx, 1, 1)
	if err == nil || !strings.Contains(err.Error(), "could not be resolved") {
		t.Errorf("expected could not be resolved error, got %v", err)
	}

	// 4. Success
	allowSecrets(t)
	got, err := signingSecret(ctx, 1, 1)
	if err != nil || got != "s" {
		t.Errorf("got %q, %v, want s", got, err)
	}
}

func TestPublish(t *testing.T) {
	allowSecrets(t)

	// Nil DB test
	Init(nil)
	Publish(1, "link.click", map[string]any{"ok": true})

	dbPath := filepath.Join(t.TempDir(), "eventbus.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&models.Webhook{}); err != nil {
		t.Fatal(err)
	}

	Init(gdb)

	// Publish with no hooks
	Publish(999, "link.click", map[string]any{"ok": true})

	// Publish with unmarshalable data
	Publish(1, "link.click", make(chan int))

	received := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Register webhook subscribed to link.click and another hook unsubscribed
	gdb.Create(&models.Webhook{
		OrgID:       1,
		Name:        "Test Hook",
		URL:         ts.URL,
		VaultItemID: 1,
		Events:      "link.click",
		Enabled:     true,
	})
	gdb.Create(&models.Webhook{
		OrgID:       1,
		Name:        "Unsubscribed Hook",
		URL:         ts.URL,
		VaultItemID: 1,
		Events:      "domain.created",
		Enabled:     true,
	})

	Publish(1, "link.click", map[string]any{"ok": true})

	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("expected webhook to be delivered")
	}
}
