package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func TestSlackChannel_Metadata(t *testing.T) {
	ch := NewSlackChannel()
	if ch.Name() != "slack" {
		t.Errorf("expected 'slack', got %q", ch.Name())
	}
	if ch.DisplayName() != "Slack" {
		t.Errorf("expected 'Slack', got %q", ch.DisplayName())
	}
	schema := ch.ConfigSchema()
	if len(schema) == 0 {
		t.Errorf("expected non-empty config schema")
	}
}

func TestSlackChannel_SendSuccess(t *testing.T) {
	var receivedBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ch := NewSlackChannel()
	ch.client = ts.Client()

	rec := plugin.NotificationRecipient{
		Config: map[string]any{
			"webhook_url": ts.URL,
		},
	}
	payload := plugin.NotificationPayload{
		Title:     "Deployment Finished",
		Body:      "Version v1.2.3 deployed successfully.",
		Priority:  "high",
		EventType: "deploy.success",
	}

	err := ch.Send(context.Background(), rec, payload)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(receivedBody), &m); err != nil {
		t.Fatalf("failed to parse received JSON: %v", err)
	}
	text := m["text"]
	if text == "" {
		t.Fatalf("received empty text")
	}
}

func TestSlackChannel_DatabaseFallback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	_ = db.AutoMigrate(&models.NotificationChannel{})

	var receivedBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	SetConfigDecryptor(func(stored string) (string, bool) {
		return stored, true
	})

	cfgJSON, _ := json.Marshal(map[string]string{"webhookUrl": ts.URL})
	_ = db.Create(&models.NotificationChannel{
		OrgID:     1,
		Type:      "slack",
		Name:      "Slack Channel",
		Config:    string(cfgJSON),
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	ch := NewSlackChannel(db)
	ch.client = ts.Client()

	rec := plugin.NotificationRecipient{
		OrgID: "1",
	}
	payload := plugin.NotificationPayload{
		Title: "Test Alert",
		Body:  "Database fallback message",
	}

	err = ch.Send(context.Background(), rec, payload)
	if err != nil {
		t.Fatalf("Send with db fallback failed: %v", err)
	}
	if receivedBody == "" {
		t.Fatalf("expected server to receive body")
	}
}

func TestSlackChannel_MissingWebhookURL(t *testing.T) {
	ch := NewSlackChannel()
	err := ch.Send(context.Background(), plugin.NotificationRecipient{}, plugin.NotificationPayload{Body: "hi"})
	if err == nil {
		t.Fatal("expected error on missing webhook url, got nil")
	}
}
