package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestLegacyNotifierAdapter(t *testing.T) {
	var calledCfg, calledText string
	fn := func(ctx context.Context, cfgJSON, text string) error {
		calledCfg = cfgJSON
		calledText = text
		if text == "fail" {
			return errors.New("deliberate failure")
		}
		return nil
	}

	adapter := NewLegacyNotifierAdapter("my_channel", "My Channel", fn)
	if adapter.Name() != "my_channel" {
		t.Errorf("Name() = %q, want my_channel", adapter.Name())
	}
	if adapter.DisplayName() != "My Channel" {
		t.Errorf("DisplayName() = %q, want My Channel", adapter.DisplayName())
	}
	if adapter.ConfigSchema() != nil {
		t.Errorf("ConfigSchema() should be nil")
	}

	// 1. Send with _raw_config
	rec := plugin.NotificationRecipient{
		UserID: "1",
		Config: map[string]interface{}{
			"_raw_config": `{"foo":"bar"}`,
		},
	}
	payload := plugin.NotificationPayload{
		Title: "Hello",
		Body:  "World",
	}
	if err := adapter.Send(context.Background(), rec, payload); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if calledCfg != `{"foo":"bar"}` || calledText != "World" {
		t.Errorf("got cfg=%q text=%q, want `{\"foo\":\"bar\"}` and World", calledCfg, calledText)
	}

	// 2. Send with structured map config and empty body (falls back to Title)
	rec2 := plugin.NotificationRecipient{
		UserID: "2",
		Config: map[string]interface{}{
			"key": "val",
		},
	}
	payload2 := plugin.NotificationPayload{
		Title: "Only Title",
	}
	if err := adapter.Send(context.Background(), rec2, payload2); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if calledCfg != `{"key":"val"}` || calledText != "Only Title" {
		t.Errorf("got cfg=%q text=%q", calledCfg, calledText)
	}

	// 3. Error case
	payload3 := plugin.NotificationPayload{
		Body: "fail",
	}
	if err := adapter.Send(context.Background(), rec, payload3); err == nil {
		t.Error("expected error, got nil")
	}

	// 4. Nil function case
	nilAdapter := &LegacyNotifierAdapter{ChannelName: "nil_ch"}
	if err := nilAdapter.Send(context.Background(), rec, payload); err == nil {
		t.Error("expected error for nil SendFunc, got nil")
	}

	// 5. Default title derived from name
	adapterDefaultTitle := NewLegacyNotifierAdapter("slack", "", fn)
	if adapterDefaultTitle.DisplayName() != "Slack" {
		t.Errorf("expected Slack, got %q", adapterDefaultTitle.DisplayName())
	}
	adapterEmptyTitle := &LegacyNotifierAdapter{ChannelName: "raw"}
	if adapterEmptyTitle.DisplayName() != "raw" {
		t.Errorf("expected raw, got %q", adapterEmptyTitle.DisplayName())
	}
}
