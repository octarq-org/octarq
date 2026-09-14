package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// setPassthroughDecryptor registers a decryptor that returns the stored config
// unchanged, so delivery-focused tests can hand plaintext JSON to Send without
// also testing the decrypt seam. Reset happens via t.Cleanup.
func setPassthroughDecryptor(t *testing.T) {
	t.Helper()
	SetConfigDecryptor(func(stored string) (string, bool) { return stored, true })
	t.Cleanup(func() { SetConfigDecryptor(nil) })
}

func TestSendUnknownType(t *testing.T) {
	setPassthroughDecryptor(t)
	if err := Send(context.Background(), "carrierpigeon", "{}", "x"); err == nil {
		t.Fatal("expected error for unknown channel type")
		return
	}
}

func TestRegisterProvider(t *testing.T) {
	setPassthroughDecryptor(t)
	var gotCfg, gotText string
	Register("Pigeon", func(_ context.Context, cfgJSON, text string) error {
		gotCfg, gotText = cfgJSON, text
		return nil
	})
	// Registration is case-insensitive and reachable through Send.
	if err := Send(context.Background(), "pigeon", `{"coop":1}`, "fly"); err != nil {
		t.Fatalf("Send to registered provider: %v", err)
	}
	if gotCfg != `{"coop":1}` || gotText != "fly" {
		t.Fatalf("provider got cfg=%q text=%q", gotCfg, gotText)
	}
	// A built-in type still resolves to its handler, not the registry.
	if err := Send(context.Background(), "webhook", `{}`, "x"); err == nil {
		t.Fatal("expected error: webhook with empty url")
		return
	}
}

func TestSendTelegramMissingCreds(t *testing.T) {
	setPassthroughDecryptor(t)
	if err := Send(context.Background(), "telegram", `{}`, "x"); err == nil {
		t.Fatal("expected error when telegram credentials are missing")
		return
	}
}

func TestSendWebhookDeliversText(t *testing.T) {
	setPassthroughDecryptor(t)
	var gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(body, &m)
		gotText = m.Text
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfgJSON, _ := json.Marshal(map[string]string{"url": srv.URL})
	if err := Send(context.Background(), "webhook", string(cfgJSON), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotText != "hello" {
		t.Errorf("text = %q want hello", gotText)
	}
}

func TestSendWebhookMissingURL(t *testing.T) {
	setPassthroughDecryptor(t)
	if err := Send(context.Background(), "webhook", `{}`, "x"); err == nil {
		t.Fatal("expected error when webhook url is missing")
		return
	}
}

func TestSendWebhookErrorsOnBadStatus(t *testing.T) {
	setPassthroughDecryptor(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfgJSON, _ := json.Marshal(map[string]string{"url": srv.URL})
	if err := Send(context.Background(), "webhook", string(cfgJSON), "x"); err == nil {
		t.Fatal("expected error on non-2xx webhook response")
		return
	}
}

func TestSendTelegramAPIErrors(t *testing.T) {
	setPassthroughDecryptor(t)
	// Invalid token format triggers telegram.New error
	cfgJSON := `{"botToken":"invalid-token","chatId":"123456"}`
	err := Send(context.Background(), "telegram", cfgJSON, "hello")
	if err == nil {
		t.Error("expected error for invalid telegram bot token, got nil")
	}

	// Valid format token progresses past telegram.New to notifier.Send
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cfgValidFormat := `{"botToken":"123456789:ABCdefGHIjklMNOpqrsTUVwxyz","chatId":"123456"}`
	errSend := Send(ctx, "telegram", cfgValidFormat, "hello")
	if errSend == nil {
		t.Error("expected error contacting telegram API with fake token, got nil")
	}
}

func TestSendInvalidJSON(t *testing.T) {
	setPassthroughDecryptor(t)
	if err := Send(context.Background(), "telegram", `invalid-json`, "x"); err == nil {
		t.Fatal("expected error for malformed telegram config JSON")
	}
	if err := Send(context.Background(), "webhook", `invalid-json`, "x"); err == nil {
		t.Fatal("expected error for malformed webhook config JSON")
	}
}

func TestSendTelegram_InvalidChatIDAndMissing(t *testing.T) {
	setPassthroughDecryptor(t)
	// Invalid chatId (not an int)
	cfgJSON := `{"botToken":"tok","chatId":"not-a-number"}`
	if err := Send(context.Background(), "telegram", cfgJSON, "hello"); err == nil {
		t.Error("expected error for non-integer chatId")
	}

	// Missing botToken
	if err := Send(context.Background(), "telegram", `{"botToken":"","chatId":"123"}`, "hello"); err == nil {
		t.Error("expected error for empty botToken")
	}

	// Missing chatId
	if err := Send(context.Background(), "telegram", `{"botToken":"tok","chatId":""}`, "hello"); err == nil {
		t.Error("expected error for empty chatId")
	}
}

func TestSendWebhook_InvalidScheme(t *testing.T) {
	setPassthroughDecryptor(t)
	cfgJSON := `{"url":"ftp://example.com/webhook"}`
	if err := Send(context.Background(), "webhook", cfgJSON, "hello"); err == nil {
		t.Error("expected error for ftp scheme")
	}

	// Test request execution error (canceled context)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	validURLCfg := `{"url":"http://127.0.0.1:59999/webhook"}`
	if err := Send(ctx, "webhook", validURLCfg, "hello"); err == nil {
		t.Error("expected error with canceled context")
	}
}
