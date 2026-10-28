package mail

import (
	"testing"
)

func TestSMTPSender(t *testing.T) {
	sender := NewCustomSender("127.0.0.1", "9999", "user", "pass", "system@example.com")

	msg := Message{
		To:      []string{"user@example.com"},
		Subject: "Test Outbound Mail",
		Text:    "Hello World Plaintext",
	}

	err := sender.Send(msg)
	if err == nil {
		t.Error("expected error dialing mock address, got nil")
	}

	// Test HTML mail format
	htmlMsg := Message{
		From:    "custom@example.com",
		To:      []string{"user@example.com"},
		Subject: "Test Outbound HTML",
		HTML:    "<h1>Hello World HTML</h1>",
	}
	err = sender.Send(htmlMsg)
	if err == nil {
		t.Error("expected error dialing mock address for HTML mail, got nil")
	}
}

func TestResendSender(t *testing.T) {
	s := NewResendSender("re_test_dummy_key", "from@example.com")
	if s == nil {
		t.Fatal("expected non-nil ResendSender")
	}

	msg := Message{
		To:      []string{"to@example.com"},
		Subject: "Resend Subject",
		Text:    "Resend Body Text",
	}

	// Sending without a valid API key will fail against real API, but exercises payload formatting
	_ = s.Send(msg)

	msgHTML := Message{
		From:    "custom@example.com",
		To:      []string{"to@example.com"},
		Subject: "Resend HTML",
		HTML:    "<p>Hello</p>",
	}
	_ = s.Send(msgHTML)
}
