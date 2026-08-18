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
	SetSecretDecryptor(func(stored string) (string, bool) { return stored, true })
	t.Cleanup(func() { SetSecretDecryptor(nil) })

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

func TestDeliverErrors(t *testing.T) {
	// 1. Invalid URL scheme
	deliver(context.Background(), "ftp://example.com/hook", "secret", []byte("{}"))

	// 2. Secret decryptor failure
	SetSecretDecryptor(func(stored string) (string, bool) { return "", false })
	deliver(context.Background(), "http://example.com/hook", "secret", []byte("{}"))

	// 3. No secret decryptor registered
	SetSecretDecryptor(nil)
	deliver(context.Background(), "http://example.com/hook", "secret", []byte("{}"))

	// 4. Server returns HTTP 500
	SetSecretDecryptor(func(stored string) (string, bool) { return stored, true })
	t.Cleanup(func() { SetSecretDecryptor(nil) })

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	deliver(context.Background(), ts.URL, "secret", []byte("{}"))
}

func TestSigningSecret(t *testing.T) {
	// 1. Nil decryptor
	SetSecretDecryptor(nil)
	_, err := signingSecret("raw")
	if err == nil || !strings.Contains(err.Error(), "no secret decryptor registered") {
		t.Errorf("expected no decryptor error, got %v", err)
	}

	// 2. Decryptor failure
	SetSecretDecryptor(func(s string) (string, bool) { return "", false })
	_, err = signingSecret("raw")
	if err == nil || !strings.Contains(err.Error(), "could not be decrypted") {
		t.Errorf("expected could not be decrypted error, got %v", err)
	}

	// 3. Success
	SetSecretDecryptor(func(s string) (string, bool) { return "plaintext-" + s, true })
	defer SetSecretDecryptor(nil)
	got, err := signingSecret("abc")
	if err != nil || got != "plaintext-abc" {
		t.Errorf("got %q, %v, want plaintext-abc", got, err)
	}
}

func TestPublish(t *testing.T) {
	SetSecretDecryptor(func(stored string) (string, bool) { return stored, true })
	t.Cleanup(func() { SetSecretDecryptor(nil) })

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
		OrgID:   1,
		Name:    "Test Hook",
		URL:     ts.URL,
		Secret:  "secret",
		Events:  "link.click",
		Enabled: true,
	})
	gdb.Create(&models.Webhook{
		OrgID:   1,
		Name:    "Unsubscribed Hook",
		URL:     ts.URL,
		Secret:  "secret",
		Events:  "domain.created",
		Enabled: true,
	})

	Publish(1, "link.click", map[string]any{"ok": true})

	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("expected webhook to be delivered")
	}
}
