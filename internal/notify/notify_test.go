package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendUnknownType(t *testing.T) {
	if err := Send(context.Background(), "carrierpigeon", "{}", "x"); err == nil {
		t.Fatal("expected error for unknown channel type")
	}
}

func TestRegisterProvider(t *testing.T) {
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
	}
}

func TestSendTelegramMissingCreds(t *testing.T) {
	if err := Send(context.Background(), "telegram", `{}`, "x"); err == nil {
		t.Fatal("expected error when telegram credentials are missing")
	}
}

func TestSendWebhookDeliversText(t *testing.T) {
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
	if err := Send(context.Background(), "webhook", `{}`, "x"); err == nil {
		t.Fatal("expected error when webhook url is missing")
	}
}

func TestSendWebhookErrorsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfgJSON, _ := json.Marshal(map[string]string{"url": srv.URL})
	if err := Send(context.Background(), "webhook", string(cfgJSON), "x"); err == nil {
		t.Fatal("expected error on non-2xx webhook response")
	}
}

func TestSendTelegramAPIErrors(t *testing.T) {
	cfgJSON := `{"botToken":"invalid-token","chatId":"123456"}`
	err := Send(context.Background(), "telegram", cfgJSON, "hello")
	if err == nil {
		t.Error("expected error for invalid telegram bot token, got nil")
	}
}

func TestSendInvalidJSON(t *testing.T) {
	if err := Send(context.Background(), "telegram", `invalid-json`, "x"); err == nil {
		t.Fatal("expected error for malformed telegram config JSON")
	}
	if err := Send(context.Background(), "webhook", `invalid-json`, "x"); err == nil {
		t.Fatal("expected error for malformed webhook config JSON")
	}
}
