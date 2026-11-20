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
		return
	}
}

func TestOpenWithErrors(t *testing.T) {
	var key [32]byte
	copy(key[:], []byte("01234567890123456789012345678901"))

	if _, err := openWith(key, "not-valid-base64!!!", nil); err == nil {
		t.Error("expected error for invalid base64")
	}

	shortB64 := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := openWith(key, shortB64, nil); err == nil {
		t.Error("expected error for short ciphertext")
	}
}

func TestUnwrapDEK_WrongLength(t *testing.T) {
	c := New("master-key")
	shortPayload := []byte("0123456789012345")
	wrapped, err := sealWith(c.kek, shortPayload, nil)
	if err != nil {
		t.Fatalf("sealWith: %v", err)
	}

	_, err = c.unwrapKey(wrapped)
	if err == nil || !strings.Contains(err.Error(), "wrong length") {
		t.Fatalf("expected wrong length error, got %v", err)
	}
}

type errStore struct{}

func (errStore) Get(string) (string, bool)         { return "", false }
func (errStore) Set(string, string) error          { return errors.New("db disk full") }
func (errStore) Delete(string) error               { return errors.New("db disk full") }
func (errStore) HasEncryptedValues() (bool, error) { return false, nil }

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

	c := New("master")
	err := c.EnableEnvelope(newMemStore())
	if err == nil || !strings.Contains(err.Error(), "generate DEK") {
		t.Errorf("expected generate DEK error, got %v", err)
	}

	var key [32]byte
	_, err = sealWith(key, []byte("payload"), nil)
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

	originalPlaintext := []byte("confidential business record")
	cipherText, err := cOld.Encrypt(originalPlaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	wrappedOld, ok := store.Get(DEKSettingKey)
	if !ok {
		t.Fatal("dek not found in store")
	}
	dek, err := cOld.unwrapKey(wrappedOld)
	if err != nil {
		t.Fatalf("unwrapKey with old master: %v", err)
	}
	cNew := New("new-master-secret")
	wrappedNew, err := sealWith(cNew.kek, dek[:], nil)
	if err != nil {
		t.Fatalf("sealWith new master: %v", err)
	}
	if err := store.Set(DEKSettingKey, wrappedNew); err != nil {
		t.Fatalf("store.Set: %v", err)
	}

	cRotated := New("new-master-secret")
	if err := cRotated.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope new: %v", err)
	}

	decrypted, err := cRotated.Decrypt(cipherText)
	if err != nil {
		t.Fatalf("Decrypt after key rotation failed: %v", err)
	}
	if string(decrypted) != string(originalPlaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, originalPlaintext)
	}
}
