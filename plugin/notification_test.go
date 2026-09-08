package plugin

import (
	"context"
	"encoding/json"
	"testing"
)

type dummyChannel struct {
	name   string
	disp   string
	schema string
	sent   []NotificationPayload
}

func (d *dummyChannel) Name() string        { return d.name }
func (d *dummyChannel) DisplayName() string { return d.disp }
func (d *dummyChannel) ConfigSchema() json.RawMessage {
	if d.schema == "" {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(d.schema)
}
func (d *dummyChannel) Send(ctx context.Context, recipient NotificationRecipient, payload NotificationPayload) error {
	d.sent = append(d.sent, payload)
	return nil
}

func TestNotificationChannel_InterfaceAndRegistration(t *testing.T) {
	ch := &dummyChannel{
		name:   "test_channel",
		disp:   "Test Channel",
		schema: `{"type":"object"}`,
	}

	if ch.Name() != "test_channel" {
		t.Errorf("expected test_channel, got %s", ch.Name())
	}
	if ch.DisplayName() != "Test Channel" {
		t.Errorf("expected Test Channel, got %s", ch.DisplayName())
	}
	if string(ch.ConfigSchema()) != `{"type":"object"}` {
		t.Errorf("unexpected schema: %s", string(ch.ConfigSchema()))
	}

	// Register with nil channel -> error
	if err := RegisterNotificationChannel(nil, nil); err == nil {
		t.Error("expected error when registering nil channel")
	}

	// Register with empty name -> error
	emptyNameCh := &dummyChannel{name: ""}
	if err := RegisterNotificationChannel(nil, emptyNameCh); err == nil {
		t.Error("expected error when registering channel with empty name")
	}

	// Register with nil context -> succeeds without panic
	if err := RegisterNotificationChannel(nil, ch); err != nil {
		t.Errorf("unexpected error with nil context: %v", err)
	}

	// Register with context where RegisterNotificationChannel is wired
	var registered NotificationChannel
	ctx := &Context{
		RegisterNotificationChannel: func(c NotificationChannel) {
			registered = c
		},
	}
	if err := RegisterNotificationChannel(ctx, ch); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if registered != ch {
		t.Errorf("expected registered channel to be ch, got %v", registered)
	}

	// Test Send
	payload := NotificationPayload{
		EventType: "cron.job_failed",
		Title:     "Cron Job Failed",
		Body:      "Backup job timed out",
		Priority:  "high",
		Data:      map[string]interface{}{"jobId": "backup-daily"},
		UserID:    "user-1",
		OrgID:     "org-1",
	}
	recipient := NotificationRecipient{
		UserID: "user-1",
		OrgID:  "org-1",
		Config: map[string]interface{}{"endpoint": "https://example.com"},
	}

	if err := ch.Send(context.Background(), recipient, payload); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if len(ch.sent) != 1 {
		t.Fatalf("expected 1 sent payload, got %d", len(ch.sent))
	}
	if ch.sent[0].EventType != "cron.job_failed" {
		t.Errorf("expected cron.job_failed, got %s", ch.sent[0].EventType)
	}

	// Test JSON serialization of payload and recipient
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	var unmarshaledPayload NotificationPayload
	if err := json.Unmarshal(b, &unmarshaledPayload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if unmarshaledPayload.Title != "Cron Job Failed" {
		t.Errorf("title mismatch: %s", unmarshaledPayload.Title)
	}

	rb, err := json.Marshal(recipient)
	if err != nil {
		t.Fatalf("marshal recipient failed: %v", err)
	}
	var unmarshaledRecipient NotificationRecipient
	if err := json.Unmarshal(rb, &unmarshaledRecipient); err != nil {
		t.Fatalf("unmarshal recipient failed: %v", err)
	}
	if unmarshaledRecipient.UserID != "user-1" {
		t.Errorf("userId mismatch: %s", unmarshaledRecipient.UserID)
	}
}
