package eventbus

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
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

	deliver(ctx, ts.URL, secret, payload)

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

	// Verify HMAC
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

func TestPublish(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&models.Webhook{}); err != nil {
		t.Fatal(err)
	}

	received := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Register webhook
	hook := models.Webhook{
		OrgID:   1,
		Name:    "Test Hook",
		URL:     ts.URL,
		Secret:  "secret",
		Events:  "link.click",
		Enabled: true,
	}
	gdb.Create(&hook)

	Init(gdb)

	Publish(1, "link.click", map[string]any{"ok": true})

	select {
	case <-received:
	case <-time.After(1 * time.Second):
		t.Error("expected webhook to be delivered")
	}
}
