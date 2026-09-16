package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func TestWebhookChannel_Basics(t *testing.T) {
	ch := NewWebhookChannel()
	if ch.Name() != "webhook" {
		t.Errorf("Name() = %q, want webhook", ch.Name())
	}
	if ch.DisplayName() != "Webhook" {
		t.Errorf("DisplayName() = %q, want Webhook", ch.DisplayName())
	}
	schema := ch.ConfigSchema()
	if len(schema) == 0 || !strings.Contains(string(schema), "url") {
		t.Errorf("unexpected ConfigSchema: %s", string(schema))
	}
}

func TestWebhookChannel_DeliveryAndErrors(t *testing.T) {
	var gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(body, &m)
		gotText = m.Text
		if r.URL.Path == "/error" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewWebhookChannel()

	// 1. Success delivery via map config
	err := ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"url": srv.URL},
	}, plugin.NotificationPayload{
		Body: "test message",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if gotText != "test message" {
		t.Errorf("got text %q, want 'test message'", gotText)
	}

	// 2. Success delivery via _raw_config and Title-only fallback
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"_raw_config": `{"url":"` + srv.URL + `"}`},
	}, plugin.NotificationPayload{
		Title: "test title only",
	})
	if err != nil {
		t.Fatalf("Send with _raw_config failed: %v", err)
	}
	if gotText != "test title only" {
		t.Errorf("got text %q, want 'test title only'", gotText)
	}

	// 3. Server returns HTTP 500 error
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"url": srv.URL + "/error"},
	}, plugin.NotificationPayload{Body: "msg"})
	if err == nil || !strings.Contains(err.Error(), "webhook: HTTP 500") {
		t.Errorf("expected HTTP 500 error, got %v", err)
	}

	// 4. Missing webhook url
	err = ch.Send(context.Background(), plugin.NotificationRecipient{}, plugin.NotificationPayload{Body: "msg"})
	if err == nil || !strings.Contains(err.Error(), "missing webhook url") {
		t.Errorf("expected missing webhook url error, got %v", err)
	}

	// 5. Invalid JSON in _raw_config
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"_raw_config": "invalid-json"},
	}, plugin.NotificationPayload{Body: "msg"})
	if err == nil {
		t.Error("expected error for malformed _raw_config, got nil")
	}

	// 6. Invalid URL scheme (ftp)
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"url": "ftp://example.com/hook"},
	}, plugin.NotificationPayload{Body: "msg"})
	if err == nil {
		t.Error("expected error for ftp scheme, got nil")
	}

	// 7. Bad URL character
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"url": "http://\x7f/invalid"},
	}, plugin.NotificationPayload{Body: "msg"})
	if err == nil {
		t.Error("expected error for bad url, got nil")
	}

	// 8. Canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = ch.Send(ctx, plugin.NotificationRecipient{
		Config: map[string]any{"url": srv.URL},
	}, plugin.NotificationPayload{Body: "msg"})
	if err == nil {
		t.Error("expected error with canceled context, got nil")
	}
}

func TestWebhookChannel_DBLookup(t *testing.T) {
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

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_ = db.AutoMigrate(&models.NotificationChannel{})

	SetConfigDecryptor(func(stored string) (string, bool) {
		return stored, true
	})
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	// Insert active webhook channel for org 1
	db.Create(&models.NotificationChannel{
		OrgID:   1,
		Type:    "webhook",
		Config:  `{"url":"` + srv.URL + `"}`,
		Enabled: true,
	})

	ch := NewWebhookChannel(db)
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "1",
	}, plugin.NotificationPayload{Body: "from db lookup"})
	if err != nil {
		t.Fatalf("Send with DB lookup failed: %v", err)
	}
	if gotText != "from db lookup" {
		t.Errorf("got text %q, want 'from db lookup'", gotText)
	}

	// Org 2 has no channel configured -> missing webhook url
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "2",
	}, plugin.NotificationPayload{Body: "msg"})
	if err == nil || !strings.Contains(err.Error(), "missing webhook url") {
		t.Errorf("expected missing webhook url for org 2, got %v", err)
	}
}
