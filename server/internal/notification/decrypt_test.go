package notification

import "testing"

func TestConfigPlaintext(t *testing.T) {
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	// 1. No decryptor registered
	SetConfigDecryptor(nil)
	if _, err := ConfigPlaintext("encrypted"); err == nil {
		t.Error("expected error when no decryptor registered, got nil")
	}

	// 2. Decryptor returns false
	SetConfigDecryptor(func(s string) (string, bool) {
		return "", false
	})
	if _, err := ConfigPlaintext("garbled"); err == nil {
		t.Error("expected error when decrypt returns false, got nil")
	}

	// 3. Decryptor succeeds
	SetConfigDecryptor(func(s string) (string, bool) {
		return "plain-" + s, true
	})
	got, err := ConfigPlaintext("secret")
	if err != nil {
		t.Fatalf("ConfigPlaintext failed: %v", err)
	}
	if got != "plain-secret" {
		t.Errorf("got %q, want %q", got, "plain-secret")
	}
}
