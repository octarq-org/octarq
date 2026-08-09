package notify

import "testing"

func TestConfigPlaintextDecrypt(t *testing.T) {
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	const plaintextRow = `{"botToken":"legacy-token","chatId":"42"}`
	const cipherRow = "enc:" + plaintextRow

	SetConfigDecryptor(func(stored string) (string, bool) {
		if len(stored) > 4 && stored[:4] == "enc:" {
			return stored[4:], true
		}
		return "", false
	})

	pt, err := configPlaintext(cipherRow)
	if err != nil || pt != plaintextRow {
		t.Errorf("encrypted row: got %q err %v, want %q", pt, err, plaintextRow)
	}

	if _, err := configPlaintext(plaintextRow); err == nil {
		t.Errorf("legacy plaintext row must error, got nil")
	}

	SetConfigDecryptor(nil)
	if _, err := configPlaintext(cipherRow); err == nil {
		t.Errorf("no decryptor registered must error, got nil")
	}
}
