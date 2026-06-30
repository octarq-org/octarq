package crypto

import "testing"

// enveloped returns a Cipher in envelope mode backed by a fresh in-memory store.
func enveloped(t *testing.T, secret string) *Cipher {
	t.Helper()
	c := New(secret)
	if err := c.EnableEnvelope(newMemStore()); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	return c
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	c := enveloped(t, "test-secret")
	plain := []byte("hello, secret world")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", got, plain)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	c := enveloped(t, "test-secret")
	enc, err := c.Encrypt([]byte("payload"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	b := []byte(enc)
	mid := len(b) / 2
	if b[mid] == 'A' {
		b[mid] = 'B'
	} else {
		b[mid] = 'A'
	}
	if _, err := c.Decrypt(string(b)); err == nil {
		t.Fatal("expected error decrypting tampered ciphertext, got nil")
	}
}

func TestEncryptBeforeEnvelopeFails(t *testing.T) {
	c := New("test-secret")
	if _, err := c.Encrypt([]byte("x")); err == nil {
		t.Fatal("Encrypt should fail before EnableEnvelope")
	}
}

func TestSignVerify(t *testing.T) {
	// Sign/Verify use the KEK and work without the envelope being enabled.
	c := New("test-secret")
	msg := []byte("authenticate me")
	sig := c.Sign(msg)
	if !c.Verify(msg, sig) {
		t.Fatal("Verify rejected a valid signature")
	}
	if c.Verify([]byte("different message"), sig) {
		t.Fatal("Verify accepted a signature for a different message")
	}
	if c.Verify(msg, sig+"x") {
		t.Fatal("Verify accepted a tampered signature")
	}
}
