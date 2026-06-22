package crypto

import "testing"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	c := New("test-secret")
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
	c := New("test-secret")
	enc, err := c.Encrypt([]byte("payload"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Flip a character in the middle of the base64 string.
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

func TestDecryptWrongKeyFails(t *testing.T) {
	enc, err := New("secret-a").Encrypt([]byte("payload"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := New("secret-b").Decrypt(enc); err == nil {
		t.Fatal("expected error decrypting with wrong key, got nil")
	}
}

func TestSignVerify(t *testing.T) {
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
