package crypto

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

func TestEncryptWithAADRoundTrip(t *testing.T) {
	c := enveloped(t, "test-secret")
	aad := []byte("org:7/record:42")

	ct, err := c.EncryptWithAAD([]byte("card-number"), aad)
	if err != nil {
		t.Fatalf("EncryptWithAAD: %v", err)
	}
	got, err := c.DecryptWithAAD(ct, aad)
	if err != nil {
		t.Fatalf("DecryptWithAAD: %v", err)
	}
	if string(got) != "card-number" {
		t.Fatalf("DecryptWithAAD = %q, want %q", got, "card-number")
	}
}

func TestEncryptWithAADRejectsDifferentAAD(t *testing.T) {
	c := enveloped(t, "test-secret")
	ct, err := c.EncryptWithAAD([]byte("card-number"), []byte("org:7"))
	if err != nil {
		t.Fatalf("EncryptWithAAD: %v", err)
	}

	if _, err := c.DecryptWithAAD(ct, []byte("org:8")); err == nil {
		t.Fatal("DecryptWithAAD opened a ciphertext sealed with another AAD")
	}
	if _, err := c.Decrypt(ct); err == nil {
		t.Fatal("Decrypt opened a ciphertext sealed with an AAD")
	}
}

func TestNilAADMatchesEncrypt(t *testing.T) {
	c := enveloped(t, "test-secret")
	plaintext := []byte("payload")

	sealedByEncrypt, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := c.DecryptWithAAD(sealedByEncrypt, nil)
	if err != nil || string(got) != string(plaintext) {
		t.Fatalf("DecryptWithAAD(nil) = %q, %v; want %q", got, err, plaintext)
	}

	sealedWithNilAAD, err := c.EncryptWithAAD(plaintext, nil)
	if err != nil {
		t.Fatalf("EncryptWithAAD(nil): %v", err)
	}
	got, err = c.Decrypt(sealedWithNilAAD)
	if err != nil || string(got) != string(plaintext) {
		t.Fatalf("Decrypt = %q, %v; want %q", got, err, plaintext)
	}
}

func TestSubjectVaultsAreIsolated(t *testing.T) {
	c := enveloped(t, "test-secret")
	a, err := c.SubjectVault(7, "user-a")
	if err != nil {
		t.Fatalf("SubjectVault(a): %v", err)
	}
	b, err := c.SubjectVault(7, "user-b")
	if err != nil {
		t.Fatalf("SubjectVault(b): %v", err)
	}
	plaintext := []byte("same-plaintext")

	ctA, err := a.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("subject A Encrypt: %v", err)
	}
	if _, err := b.Decrypt(ctA); err == nil {
		t.Fatal("subject B decrypted subject A's ciphertext")
	}

	ctA2, err := a.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("subject A Encrypt: %v", err)
	}
	ctB, err := b.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("subject B Encrypt: %v", err)
	}
	if ctA2 == ctB {
		t.Fatal("subjects A and B produced the same ciphertext for the same plaintext")
	}

	for name, v := range map[string]*SubjectVault{"a": a, "b": b} {
		ct, err := v.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("subject %s Encrypt: %v", name, err)
		}
		got, err := v.Decrypt(ct)
		if err != nil || string(got) != string(plaintext) {
			t.Fatalf("subject %s roundtrip = %q, %v; want %q", name, got, err, plaintext)
		}
	}
}

func TestSubjectVaultsSeparateOwners(t *testing.T) {
	c := enveloped(t, "test-secret")
	one, err := c.SubjectVault(1, "inbox")
	if err != nil {
		t.Fatalf("SubjectVault(owner 1): %v", err)
	}
	two, err := c.SubjectVault(2, "inbox")
	if err != nil {
		t.Fatalf("SubjectVault(owner 2): %v", err)
	}

	ct, err := one.Encrypt([]byte("message"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := two.Decrypt(ct); err == nil {
		t.Fatal("the same subject name under another owner opened the ciphertext")
	}
}

func TestSubjectVaultIsCached(t *testing.T) {
	c := enveloped(t, "test-secret")
	first, err := c.SubjectVault(3, "cache")
	if err != nil {
		t.Fatalf("SubjectVault: %v", err)
	}
	second, err := c.SubjectVault(3, "cache")
	if err != nil {
		t.Fatalf("SubjectVault: %v", err)
	}
	if first != second {
		t.Fatal("SubjectVault returned two instances for one subject")
	}
}

func TestSubjectVaultPersistsAcrossProcesses(t *testing.T) {
	store := newMemStore()
	c1 := New("test-secret")
	if err := c1.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope c1: %v", err)
	}
	a1, err := c1.SubjectVault(9, "persisted")
	if err != nil {
		t.Fatalf("SubjectVault c1: %v", err)
	}
	ct, err := a1.Encrypt([]byte("still-readable"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	c2 := New("test-secret")
	if err := c2.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope c2: %v", err)
	}
	a2, err := c2.SubjectVault(9, "persisted")
	if err != nil {
		t.Fatalf("SubjectVault c2: %v", err)
	}
	got, err := a2.Decrypt(ct)
	if err != nil || string(got) != "still-readable" {
		t.Fatalf("reloaded subject vault = %q, %v; want %q", got, err, "still-readable")
	}
}

func TestShredSubjectKey(t *testing.T) {
	c := enveloped(t, "test-secret")
	shredded, err := c.SubjectVault(5, "erase")
	if err != nil {
		t.Fatalf("SubjectVault(shredded): %v", err)
	}
	other, err := c.SubjectVault(5, "keep")
	if err != nil {
		t.Fatalf("SubjectVault(other): %v", err)
	}

	shreddedCT, err := shredded.Encrypt([]byte("gone"))
	if err != nil {
		t.Fatalf("shredded Encrypt: %v", err)
	}
	otherCT, err := other.Encrypt([]byte("kept"))
	if err != nil {
		t.Fatalf("other Encrypt: %v", err)
	}
	instanceCT, err := c.Encrypt([]byte("instance"))
	if err != nil {
		t.Fatalf("instance Encrypt: %v", err)
	}

	if err := c.ShredSubjectKey(5, "erase"); err != nil {
		t.Fatalf("ShredSubjectKey: %v", err)
	}
	if err := c.ShredSubjectKey(5, "erase"); err != nil {
		t.Fatalf("second ShredSubjectKey: %v", err)
	}

	revived, err := c.SubjectVault(5, "erase")
	if err != nil {
		t.Fatalf("SubjectVault after shred: %v", err)
	}
	if revived == shredded {
		t.Fatal("SubjectVault returned the cached vault of a shredded subject")
	}
	if _, err := revived.Decrypt(shreddedCT); err == nil {
		t.Fatal("a shredded subject decrypted data sealed before the shred")
	}
	// A handle the caller kept across the shred must stop working too, not only
	// a handle resolved afterwards.
	if _, err := shredded.Decrypt(shreddedCT); err == nil {
		t.Fatal("a vault handle kept across the shred decrypted data sealed before it")
	}
	if _, err := shredded.Encrypt([]byte("new")); err == nil {
		t.Fatal("a vault handle kept across the shred sealed new data")
	}
	if got, err := other.Decrypt(otherCT); err != nil || string(got) != "kept" {
		t.Fatalf("other subject = %q, %v; want %q", got, err, "kept")
	}
	if got, err := c.Decrypt(instanceCT); err != nil || string(got) != "instance" {
		t.Fatalf("instance DEK = %q, %v; want %q", got, err, "instance")
	}
}

func TestSubjectVaultResolvesNamespacesThroughParent(t *testing.T) {
	c := enveloped(t, "test-secret")
	a, err := c.SubjectVault(6, "target")
	if err != nil {
		t.Fatalf("SubjectVault: %v", err)
	}
	again, err := a.SubjectVault(6, "target")
	if err != nil {
		t.Fatalf("subject vault SubjectVault: %v", err)
	}
	if again != a {
		t.Fatal("a subject vault resolved its own namespace to another instance")
	}

	ct, err := a.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := a.ShredSubjectKey(6, "target"); err != nil {
		t.Fatalf("subject vault ShredSubjectKey: %v", err)
	}
	revived, err := c.SubjectVault(6, "target")
	if err != nil {
		t.Fatalf("SubjectVault after shred: %v", err)
	}
	if _, err := revived.Decrypt(ct); err == nil {
		t.Fatal("a shredded subject decrypted data sealed before the shred")
	}
}

func TestSubjectVaultRequiresEnvelope(t *testing.T) {
	c := New("test-secret")
	if _, err := c.SubjectVault(1, "s"); err == nil {
		t.Fatal("SubjectVault worked before EnableEnvelope")
	}
	if err := c.ShredSubjectKey(1, "s"); err == nil {
		t.Fatal("ShredSubjectKey worked before EnableEnvelope")
	}
	if _, err := c.EncryptWithAAD([]byte("x"), []byte("a")); err == nil {
		t.Fatal("EncryptWithAAD worked before EnableEnvelope")
	}
	if _, err := c.DecryptWithAAD("x", []byte("a")); err == nil {
		t.Fatal("DecryptWithAAD worked before EnableEnvelope")
	}
}

func TestSubjectVaultNeedsStore(t *testing.T) {
	c := &Cipher{ready: true}
	if _, err := c.SubjectVault(1, "s"); err == nil {
		t.Fatal("SubjectVault worked without a secret store")
	}
	if err := c.ShredSubjectKey(1, "s"); err == nil {
		t.Fatal("ShredSubjectKey worked without a secret store")
	}
}

func TestSubjectVaultRejectsBadWrappedDEK(t *testing.T) {
	store := newMemStore()
	c := New("test-secret")
	if err := c.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	key := SubjectDEKSettingKey(1, "subject")

	if err := store.Set(key, "not-valid-base64!!!"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	if _, err := c.SubjectVault(1, "subject"); err == nil || !strings.Contains(err.Error(), "cannot unwrap subject DEK") {
		t.Fatalf("corrupt wrapped DEK error = %v, want the unwrap error", err)
	}

	if err := store.Set(key, ""); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	if _, err := c.SubjectVault(1, "subject"); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("empty wrapped DEK error = %v, want the empty error", err)
	}
}

func TestSubjectVaultReportsEntropyFailure(t *testing.T) {
	store := newMemStore()
	c := New("test-secret")
	if err := c.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}

	oldReader := rand.Reader
	t.Cleanup(func() { rand.Reader = oldReader })
	rand.Reader = failReader{}

	if _, err := c.SubjectVault(1, "s"); err == nil || !strings.Contains(err.Error(), "generate subject DEK") {
		t.Fatalf("entropy failure error = %v, want the generate error", err)
	}
}

// subjectFailStore fails subject-key writes and deletes while the instance DEK
// still persists.
type subjectFailStore struct {
	memStore
	failSet    bool
	failDelete bool
}

func (s *subjectFailStore) Set(k, v string) error {
	if s.failSet && strings.HasPrefix(k, SubjectDEKKeyPrefix) {
		return errors.New("disk full")
	}
	return s.memStore.Set(k, v)
}

func (s *subjectFailStore) Delete(k string) error {
	if s.failDelete && strings.HasPrefix(k, SubjectDEKKeyPrefix) {
		return errors.New("disk full")
	}
	return s.memStore.Delete(k)
}

func TestSubjectVaultReportsStoreWriteFailure(t *testing.T) {
	store := &subjectFailStore{memStore: *newMemStore(), failSet: true}
	c := New("test-secret")
	if err := c.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}

	if _, err := c.SubjectVault(1, "s"); err == nil || !strings.Contains(err.Error(), "save subject DEK") {
		t.Fatalf("write failure error = %v, want the save error", err)
	}
}

func TestShredSubjectKeyReportsStoreDeleteFailure(t *testing.T) {
	store := &subjectFailStore{memStore: *newMemStore()}
	c := New("test-secret")
	if err := c.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	if _, err := c.SubjectVault(1, "s"); err != nil {
		t.Fatalf("SubjectVault: %v", err)
	}

	store.failDelete = true
	if err := c.ShredSubjectKey(1, "s"); err == nil || !strings.Contains(err.Error(), "delete subject DEK") {
		t.Fatalf("delete failure error = %v, want the delete error", err)
	}
}

func TestSubjectDEKSettingKeyFormat(t *testing.T) {
	got := SubjectDEKSettingKey(42, "user-1")
	want := "crypto.dek.subject.42.c6c289e49e9c05b2145860387b73bcb1"
	if got != want {
		t.Fatalf("SubjectDEKSettingKey = %q, want %q", got, want)
	}

	// The hash keeps the key length fixed for any subject string.
	long := SubjectDEKSettingKey(42, strings.Repeat("x", 200))
	if len(long) != len(want) {
		t.Fatalf("key length for a long subject = %d, want %d", len(long), len(want))
	}
}
