package notify

import (
	"context"
	"strings"
	"testing"
)

func TestDescriptorsAndRegisterWithDescriptor(t *testing.T) {
	RegisterWithDescriptor(Descriptor{Type: ""}, func(ctx context.Context, cfgJSON, text string) error { return nil })
	RegisterWithDescriptor(Descriptor{Type: "invalid"}, nil)

	customDesc := Descriptor{
		Type:        "slack",
		Title:       "Slack",
		Description: "Deliver to Slack webhook",
		Icon:        "slack",
		PluginName:  "slack-plugin",
	}
	RegisterWithDescriptor(customDesc, func(ctx context.Context, cfgJSON, text string) error {
		return nil
	})

	list := Descriptors()
	if len(list) < 3 {
		t.Fatalf("expected at least 3 descriptors, got %d", len(list))
	}

	for i := 1; i < len(list); i++ {
		if list[i-1].Type > list[i].Type {
			t.Errorf("descriptors not sorted: %s > %s", list[i-1].Type, list[i].Type)
		}
	}

	foundSlack := false
	for _, d := range list {
		if d.Type == "slack" {
			foundSlack = true
			if d.Title != "Slack" || d.PluginName != "slack-plugin" {
				t.Errorf("unexpected descriptor: %+v", d)
			}
		}
	}
	if !foundSlack {
		t.Error("slack descriptor not found in Descriptors()")
	}
}

func TestSendTelegramInvalidChatID(t *testing.T) {
	cfgJSON := `{"botToken":"valid-looking-token","chatId":"not-a-number"}`
	err := Send(context.Background(), "telegram", cfgJSON, "hello")
	if err == nil {
		t.Fatal("expected error for non-numeric telegram chatId, got nil")
		return
	}
	if !strings.Contains(err.Error(), "invalid telegram chatId") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSendWebhookInvalidScheme(t *testing.T) {
	cfgJSON := `{"url":"ftp://example.com/webhook"}`
	err := Send(context.Background(), "webhook", cfgJSON, "hello")
	if err == nil {
		t.Fatal("expected error for unsupported ftp scheme, got nil")
		return
	}
}

func TestSendWebhookBadURL(t *testing.T) {
	cfgJSON := `{"url":"http://\x7f/invalid"}`
	err := Send(context.Background(), "webhook", cfgJSON, "hello")
	if err == nil {
		t.Fatal("expected error for invalid URL characters, got nil")
		return
	}
}
