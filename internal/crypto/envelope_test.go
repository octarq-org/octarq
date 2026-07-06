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

func TestWrongMasterFailsToUnwrap(t *testing.T) {
	store := newMemStore()
	if err := New("right").EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	// A different master cannot unwrap the DEK.
	if err := New("wrong").EnableEnvelope(store); err == nil {
		t.Fatal("expected EnableEnvelope to fail unwrapping the DEK with the wrong master")
	}
}
