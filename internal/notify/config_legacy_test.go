package notify

import "testing"

// P2-12 deliberately ships no data migration: existing rows stay plaintext and
// become ciphertext the next time they are edited. That decision rests entirely
// on the read path tolerating both, so pin it. If this regresses, every channel
// configured before the upgrade stops delivering — silently, because delivery
// failures are logged and discarded, not surfaced.
func TestConfigPlaintextAcceptsBothEncodings(t *testing.T) {
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	const plaintextRow = `{"botToken":"legacy-token","chatId":"42"}`
	const cipherRow = "enc:" + plaintextRow

	SetConfigDecryptor(func(stored string) (string, bool) {
		if len(stored) > 4 && stored[:4] == "enc:" {
			return stored[4:], true
		}
		return "", false // not ciphertext — caller must fall back
	})

	if got := configPlaintext(cipherRow); got != plaintextRow {
		t.Errorf("encrypted row: got %q, want %q", got, plaintextRow)
	}
	if got := configPlaintext(plaintextRow); got != plaintextRow {
		t.Errorf("legacy plaintext row must survive untouched: got %q, want %q", got, plaintextRow)
	}

	// A build with no decryptor registered (a plugin host that never called
	// SetConfigDecryptor) must still deliver for plaintext rows.
	SetConfigDecryptor(nil)
	if got := configPlaintext(plaintextRow); got != plaintextRow {
		t.Errorf("no decryptor registered: got %q, want %q", got, plaintextRow)
	}
}
