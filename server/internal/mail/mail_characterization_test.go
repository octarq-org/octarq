package mail

import (
	"strings"
	"testing"
)

// TestCharacterization_RFC822ParseLocks locks the RFC822 parsing behavior
// including subject, sender, and text/html body extraction.
func TestCharacterization_RFC822ParseLocks(t *testing.T) {
	raw := []byte("From: sender@example.com\r\n" +
		"To: receiver@example.com\r\n" +
		"Subject: Important Notification\r\n" +
		"Message-ID: <msg123@example.com>\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Hello world!\r\n")

	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.From != "sender@example.com" {
		t.Errorf("expected From sender@example.com, got %q", parsed.From)
	}
	if parsed.To != "receiver@example.com" {
		t.Errorf("expected To receiver@example.com, got %q", parsed.To)
	}
	if parsed.Subject != "Important Notification" {
		t.Errorf("expected Subject 'Important Notification', got %q", parsed.Subject)
	}
	if parsed.MessageID != "msg123@example.com" {
		t.Errorf("expected MessageID 'msg123@example.com', got %q", parsed.MessageID)
	}
	if !strings.Contains(parsed.Text, "Hello world!") {
		t.Errorf("expected Text to contain 'Hello world!', got %q", parsed.Text)
	}
}

// TestCharacterization_UnparseableFallback locks the non-dropping fallback
// behavior for completely corrupt mail bodies.
func TestCharacterization_UnparseableFallback(t *testing.T) {
	corruptRaw := []byte{0x00, 0xFF, 0xFE, 0xFD}
	parsed, err := Parse(corruptRaw)
	if err != nil {
		t.Fatalf("expected non-error fallback for corrupt raw mail, got: %v", err)
	}
	if parsed == nil {
		t.Fatal("expected non-nil parsed record")
	}
	if len(parsed.Raw) != len(corruptRaw) {
		t.Errorf("expected raw bytes preserved")
	}
}

// TestCharacterization_StripCRLFInjection locks the header injection defense.
func TestCharacterization_StripCRLFInjection(t *testing.T) {
	maliciousInput := "victim@example.com\r\nBcc: hacker@example.com\nSubject: Injected"
	cleaned := stripCRLF(maliciousInput)

	if strings.Contains(cleaned, "\r") || strings.Contains(cleaned, "\n") {
		t.Fatalf("stripCRLF failed to strip newline characters: %q", cleaned)
	}
	if cleaned != "victim@example.comBcc: hacker@example.comSubject: Injected" {
		t.Errorf("unexpected stripCRLF output: %q", cleaned)
	}
}

// TestCharacterization_SenderFailClosedLocks locks the fail-closed auth requirements.
func TestCharacterization_SenderFailClosedLocks(t *testing.T) {
	// Construct sender with credentials
	s := NewCustomSender("127.0.0.1", "25", "user", "pass", "sender@example.com")
	if s == nil {
		t.Fatal("NewSender returned nil")
	}

	msg := Message{
		From:    "sender@example.com",
		To:      []string{"rec@example.com"},
		Subject: "Test",
		Text:    "Test message",
	}

	// Attempting to send to a closed port must fail with network error, not panic
	err := s.Send(msg)
	if err == nil {
		t.Fatal("expected error connecting to inactive port")
	}
}
