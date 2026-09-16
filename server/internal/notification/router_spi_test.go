package notification

import (
	"context"
	"strings"
	"testing"
)

func TestRouter_DescriptorsAndRegistration(t *testing.T) {
	r := NewRouter(nil)

	// 1. Initial 4 built-ins should be present in Descriptors
	descs := r.Descriptors()
	if len(descs) < 4 {
		t.Fatalf("expected at least 4 descriptors, got %d", len(descs))
	}

	// Verify alphabetical sorting
	for i := 1; i < len(descs); i++ {
		if descs[i-1].Type > descs[i].Type {
			t.Errorf("descriptors not sorted: %s > %s", descs[i-1].Type, descs[i].Type)
		}
	}

	// 2. GetDescriptor for built-in
	d, ok := r.GetDescriptor("telegram")
	if !ok || d.Title != "Telegram" {
		t.Errorf("expected telegram descriptor, got ok=%v, d=%+v", ok, d)
	}

	// 3. RegisterChannel errors on nil or empty
	if err := r.RegisterChannel(nil); err == nil {
		t.Error("expected error for nil channel, got nil")
	}
	emptyCh := NewLegacyNotifierAdapter("", "", nil)
	if err := r.RegisterChannel(emptyCh); err == nil {
		t.Error("expected error for empty channel name, got nil")
	}
	if err := r.RegisterChannelWithDescriptor(nil, Descriptor{Type: "foo"}); err == nil {
		t.Error("expected error for nil channel in RegisterChannelWithDescriptor, got nil")
	}
	if err := r.RegisterChannelWithDescriptor(emptyCh, Descriptor{Type: ""}); err == nil {
		t.Error("expected error for empty name in RegisterChannelWithDescriptor, got nil")
	}

	// 4. Register custom channel with descriptor
	customCh := NewLegacyNotifierAdapter("sms", "SMS Service", func(ctx context.Context, cfgJSON, text string) error {
		return nil
	})
	customDesc := Descriptor{
		Type:        "sms",
		Title:       "SMS Gateway",
		Description: "Deliver via Twilio SMS",
		Icon:        "phone",
		PluginName:  "telecom",
	}
	if err := r.RegisterChannelWithDescriptor(customCh, customDesc); err != nil {
		t.Fatalf("RegisterChannelWithDescriptor failed: %v", err)
	}

	descAfter, ok := r.GetDescriptor("sms")
	if !ok || descAfter.Title != "SMS Gateway" || descAfter.PluginName != "telecom" {
		t.Errorf("unexpected custom descriptor: %+v", descAfter)
	}

	// 5. RegisterChannel without explicit descriptor derives defaults
	pushCh := NewLegacyNotifierAdapter("push", "Push Notification", func(ctx context.Context, cfgJSON, text string) error {
		return nil
	})
	if err := r.RegisterChannel(pushCh); err != nil {
		t.Fatalf("RegisterChannel failed: %v", err)
	}
	pushDesc, ok := r.GetDescriptor("push")
	if !ok || pushDesc.Title != "Push Notification" || pushDesc.Icon != "bell" {
		t.Errorf("unexpected derived descriptor: %+v", pushDesc)
	}

	// 6. SendDirect success and unknown type
	if err := r.SendDirect(context.Background(), "sms", `{"key":"val"}`, "hello"); err != nil {
		t.Errorf("SendDirect failed: %v", err)
	}
	if err := r.SendDirect(context.Background(), "unknown-ch", "{}", "hello"); err == nil || !strings.Contains(err.Error(), "unknown notification channel type") {
		t.Errorf("expected unknown type error, got %v", err)
	}
}
