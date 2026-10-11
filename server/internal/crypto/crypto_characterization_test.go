package crypto_test

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/crypto"
)

type memStore struct {
	data map[string]string
}

func (m *memStore) Get(key string) (string, bool) {
	v, ok := m.data[key]
	return v, ok
}

func (m *memStore) Set(key, val string) error {
	m.data[key] = val
	return nil
}

func TestCryptoCharacterizationUninitialized(t *testing.T) {
	c := crypto.New("test-master-secret-12345")

	_, err := c.Encrypt([]byte("hello"))
	if err == nil || err.Error() != "crypto: envelope not initialized" {
		t.Fatalf("expected 'crypto: envelope not initialized', got %v", err)
	}

	_, err = c.Decrypt("dGVzdA==")
	if err == nil || err.Error() != "crypto: envelope not initialized" {
		t.Fatalf("expected 'crypto: envelope not initialized', got %v", err)
	}
}

func TestCryptoCharacterizationSignVerify(t *testing.T) {
	c := crypto.New("test-master-secret-12345")
	msg := []byte("session-payload-123")
	sig := c.Sign(msg)

	if !c.Verify(msg, sig) {
		t.Fatal("expected signature to verify cleanly")
	}
	if c.Verify(msg, sig+"tampered") {
		t.Fatal("expected tampered signature to fail verification")
	}
	if c.Verify([]byte("different-payload"), sig) {
		t.Fatal("expected mismatched payload to fail verification")
	}
}

func TestCryptoCharacterizationEnvelopeRoundtrip(t *testing.T) {
	store := &memStore{data: make(map[string]string)}
	c := crypto.New("master-secret-999")

	if err := c.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope failed: %v", err)
	}

	plaintext := []byte("super-secret-database-token")
	ciphertext, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := c.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted %q != plaintext %q", string(decrypted), string(plaintext))
	}
}
