package notification

import (
	"context"
	"strings"
	"testing"
)

func TestRouter_DescriptorsAndRegistration(t *testing.T) {
	r := NewRouter(nil)

	descs := r.Descriptors()
	if len(descs) < 4 {
		t.Fatalf("expected at least 4 descriptors, got %d", len(descs))
	}

	for i := 1; i < len(descs); i++ {
		if descs[i-1].Type > descs[i].Type {
			t.Errorf("descriptors not sorted: %s > %s", descs[i-1].Type, descs[i].Type)
		}
	}

	d, ok := r.GetDescriptor("telegram")
	if !ok || d.Title != "Telegram" {
		t.Errorf("expected telegram descriptor, got ok=%v, d=%+v", ok, d)
	}

	if err := r.RegisterChannel(nil); err == nil {
		t.Error("expected error for nil channel, got nil")
	}
	emptyCh := &mockChannel{}
	if err := r.RegisterChannel(emptyCh); err == nil {
		t.Error("expected error for empty channel name, got nil")
	}
	if err := r.RegisterChannelWithDescriptor(nil, Descriptor{Type: "foo"}); err == nil {
		t.Error("expected error for nil channel in RegisterChannelWithDescriptor, got nil")
	}
	if err := r.RegisterChannelWithDescriptor(emptyCh, Descriptor{Type: ""}); err == nil {
		t.Error("expected error for empty name in RegisterChannelWithDescriptor, got nil")
	}

	customCh := &mockChannel{name: "sms", disp: "SMS Service"}
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

	pushCh := &mockChannel{name: "push", disp: "Push Notification"}
	if err := r.RegisterChannel(pushCh); err != nil {
		t.Fatalf("RegisterChannel failed: %v", err)
	}
	pushDesc, ok := r.GetDescriptor("push")
	if !ok || pushDesc.Title != "Push Notification" || pushDesc.Icon != "bell" {
		t.Errorf("unexpected derived descriptor: %+v", pushDesc)
	}

	if err := r.SendDirect(context.Background(), "sms", `{"key":"val"}`, "hello"); err != nil {
		t.Errorf("SendDirect failed: %v", err)
	}
	if err := r.SendDirect(context.Background(), "unknown-ch", "{}", "hello"); err == nil || !strings.Contains(err.Error(), "unknown notification channel type") {
		t.Errorf("expected unknown type error, got %v", err)
	}

	replacementEmail := &mockChannel{name: "email", disp: "Updated Email Service"}
	if err := r.RegisterChannel(replacementEmail); err != nil {
		t.Fatalf("RegisterChannel returned error: %v", err)
	}
	currentEmail, ok := r.GetChannel("email")
	if !ok || currentEmail != replacementEmail {
		t.Errorf("expected channel replacement to take effect, got %v", currentEmail)
	}
}
