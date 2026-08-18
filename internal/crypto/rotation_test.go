package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestDecryptBeforeEnvelopeFails(t *testing.T) {
	c := New("test-secret")
	if _, err := c.Decrypt("some-ciphertext"); err == nil {
		t.Fatal("Decrypt should fail before EnableEnvelope")
	}
}

func TestOpenWithErrors(t *testing.T) {
	var key [32]byte
	copy(key[:], []byte("01234567890123456789012345678901"))

	// 1. Invalid base64
	if _, err := openWith(key, "not-valid-base64!!!"); err == nil {
		t.Error("expected error for invalid base64")
	}

	// 2. Ciphertext too short (< nonce size 12 bytes)
	shortB64 := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := openWith(key, shortB64); err == nil {
		t.Error("expected error for short ciphertext")
	}
}

func TestUnwrapDEK_WrongLength(t *testing.T) {
	c := New("master-key")
	// Seal 16 bytes instead of 32 bytes
	shortPayload := []byte("0123456789012345")
	wrapped, err := sealWith(c.kek, shortPayload)
	if err != nil {
		t.Fatalf("sealWith: %v", err)
	}

	_, err = c.unwrapDEK(wrapped)
	if err == nil || !strings.Contains(err.Error(), "wrong length") {
		t.Fatalf("expected wrong length error, got %v", err)
	}
}

type errStore struct{}

func (errStore) Get(string) (string, bool) { return "", false }
func (errStore) Set(string, string) error  { return errors.New("db disk full") }

func TestEnableEnvelope_StoreSetError(t *testing.T) {
	c := New("master-key")
	err := c.EnableEnvelope(errStore{})
	if err == nil || !strings.Contains(err.Error(), "db disk full") {
		t.Fatalf("expected store set error, got %v", err)
	}
}

type failReader struct{}

func (failReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated entropy failure")
}

func TestRandReaderFailures(t *testing.T) {
	oldReader := rand.Reader
	t.Cleanup(func() { rand.Reader = oldReader })
	rand.Reader = failReader{}

	// 1. EnableEnvelope first-run rand failure
	c := New("master")
	err := c.EnableEnvelope(newMemStore())
	if err == nil || !strings.Contains(err.Error(), "generate DEK") {
		t.Errorf("expected generate DEK error, got %v", err)
	}

	// 2. sealWith rand failure
	var key [32]byte
	_, err = sealWith(key, []byte("payload"))
	if err == nil {
		t.Error("expected error from rand.Reader failure in sealWith, got nil")
	}
}

func TestEnvelopeKeyRotation(t *testing.T) {
	store := newMemStore()
	cOld := New("old-master-secret")
	if err := cOld.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope old: %v", err)
	}

	// Encrypt bulk data under the old master
	originalPlaintext := []byte("confidential business record")
	cipherText, err := cOld.Encrypt(originalPlaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Perform key rotation on the store:
	// 1. Read existing wrapped DEK
	wrappedOld, ok := store.Get(dekSettingKey)
	if !ok {
		t.Fatal("dek not found in store")
	}
	// 2. Unwrap DEK using old master
	dek, err := cOld.unwrapDEK(wrappedOld)
	if err != nil {
		t.Fatalf("unwrapDEK with old master: %v", err)
	}
	// 3. Re-wrap DEK under new master
	cNew := New("new-master-secret")
	wrappedNew, err := sealWith(cNew.kek, dek[:])
	if err != nil {
		t.Fatalf("sealWith new master: %v", err)
	}
	// 4. Update store with new wrapped DEK
	if err := store.Set(dekSettingKey, wrappedNew); err != nil {
		t.Fatalf("store.Set: %v", err)
	}

	// Initialize a new Cipher with the new master key
	cRotated := New("new-master-secret")
	if err := cRotated.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope new: %v", err)
	}

	// Verify old ciphertext can still be decrypted without re-encrypting the bulk data
	decrypted, err := cRotated.Decrypt(cipherText)
	if err != nil {
		t.Fatalf("Decrypt after key rotation failed: %v", err)
	}
	if string(decrypted) != string(originalPlaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, originalPlaintext)
	}
}
