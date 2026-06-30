package crypto

import (
	"sync"
	"testing"
)

// memStore is an in-memory SecretStore for envelope tests.
type memStore struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemStore() *memStore { return &memStore{m: map[string]string{}} }

func (s *memStore) Get(k string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[k]
	return v, ok
}

func (s *memStore) Set(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

func TestEnvelopePersistsDEK(t *testing.T) {
	store := newMemStore()
	c := New("master")
	if err := c.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	if _, ok := store.Get(dekSettingKey); !ok {
		t.Fatal("DEK not persisted on first run")
	}
	enc, err := c.Encrypt([]byte("token"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := c.Decrypt(enc)
	if err != nil || string(got) != "token" {
		t.Fatalf("roundtrip: got %q err %v", got, err)
	}
}

func TestEnvelopeReloadSameDEK(t *testing.T) {
	// A second instance under the same master loads the same DEK and reads data.
	store := newMemStore()
	c1 := New("master")
	if err := c1.EnableEnvelope(store); err != nil {
		t.Fatalf("c1: %v", err)
	}
	data, _ := c1.Encrypt([]byte("payload"))

	c2 := New("master")
	if err := c2.EnableEnvelope(store); err != nil {
		t.Fatalf("c2: %v", err)
	}
	got, err := c2.Decrypt(data)
	if err != nil || string(got) != "payload" {
		t.Fatalf("reload: got %q err %v", got, err)
	}
}

func TestMasterKeyRotationRewrapsDEK(t *testing.T) {
	store := newMemStore()

	c1 := New("old-master")
	if err := c1.EnableEnvelope(store); err != nil {
		t.Fatalf("c1 EnableEnvelope: %v", err)
	}
	data, _ := c1.Encrypt([]byte("payload"))
	wrappedBefore, _ := store.Get(dekSettingKey)

	// Rotate: new master with the old one supplied as the rotation key.
	c2 := New("new-master")
	if err := c2.EnableEnvelope(store, "old-master"); err != nil {
		t.Fatalf("c2 EnableEnvelope (rotation): %v", err)
	}
	if wrappedAfter, _ := store.Get(dekSettingKey); wrappedAfter == wrappedBefore {
		t.Error("DEK was not re-wrapped under the new master key")
	}
	// Data encrypted under the old instance still decrypts — it's under the DEK,
	// which survived rotation untouched.
	if got, err := c2.Decrypt(data); err != nil || string(got) != "payload" {
		t.Fatalf("data unreadable after rotation: got %q err %v", got, err)
	}

	// A fresh instance under only the new master (old key dropped) still works.
	c3 := New("new-master")
	if err := c3.EnableEnvelope(store); err != nil {
		t.Fatalf("c3 EnableEnvelope (post-rotation, old key dropped): %v", err)
	}
	if got, err := c3.Decrypt(data); err != nil || string(got) != "payload" {
		t.Fatalf("data unreadable after dropping old key: got %q err %v", got, err)
	}
}

func TestWrongMasterFailsToUnwrap(t *testing.T) {
	store := newMemStore()
	if err := New("right").EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	// A different master with no rotation key cannot unwrap the DEK.
	if err := New("wrong").EnableEnvelope(store); err == nil {
		t.Fatal("expected EnableEnvelope to fail unwrapping the DEK with the wrong master")
	}
}
