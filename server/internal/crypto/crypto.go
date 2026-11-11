// Package crypto provides AES-GCM encryption for secrets at rest and HMAC
// signing for session cookies.
//
// # Envelope encryption (key rotation)
//
// Secrets at rest are encrypted under a random Data Encryption Key (DEK). The
// DEK itself is wrapped (encrypted) under a Key Encryption Key (KEK) derived
// from OCTARQ_SECRET_KEY and stored in the settings table. Rotating OCTARQ_SECRET_KEY
// therefore only re-wraps the one DEK — the bulk data is never touched. This is
// the standard KMS/Vault envelope pattern.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

// DEKSettingKey is where the wrapped DEK lives in the settings table.
const DEKSettingKey = "crypto.dek"

// SubjectDEKKeyPrefix starts the settings keys that hold per-subject wrapped
// DEKs.
const SubjectDEKKeyPrefix = "crypto.dek.subject."

// SubjectDEKSettingKey returns the settings key that holds the wrapped DEK of
// (ownerID, subject): crypto.dek.subject.<ownerID>.<hex(sha256(subject))[:32]>.
// The hash keeps the key fixed-length for any subject string.
func SubjectDEKSettingKey(ownerID uint, subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return fmt.Sprintf("%s%d.%s", SubjectDEKKeyPrefix, ownerID, hex.EncodeToString(sum[:])[:32])
}

// SecretStore is the minimal key/value access the envelope bootstrap needs to
// persist the wrapped DEK. The app backs it with the settings table; keeping it
// an interface avoids a gorm dependency here.
//
// Delete removes key. Deleting a key that is absent is not an error: callers
// use it to reach a goal state, not to observe the row.
//
// HasEncryptedValues reports whether the backing database still holds a value
// that only the DEK can read. EnableEnvelope asks before it creates a DEK, so a
// lost key does not silently replace the one that decrypts saved data.
type SecretStore interface {
	Get(key string) (string, bool)
	Set(key, val string) error
	Delete(key string) error
	HasEncryptedValues() (bool, error)
}

// ErrDEKMissing reports that the wrapped DEK is absent while saved ciphertext
// remains. A fresh DEK would make every stored value unreadable.
var ErrDEKMissing = errors.New("crypto: the key that decrypts saved values is missing. Restore the database backup taken before the upgrade, or set the affected values again: notification channels, webhook secrets, two-factor authentication, and OAuth client secrets")

// Cipher encrypts secrets at rest (under the DEK) and signs session cookies
// (under a key derived from the master secret for that purpose alone). The KEK
// is derived from OCTARQ_SECRET_KEY; the DEK is loaded by EnableEnvelope. Until
// then only Sign/Verify are usable.
type Cipher struct {
	kek     [32]byte
	mac     [32]byte
	dek     [32]byte
	ready   bool // DEK loaded (EnableEnvelope ran)
	created bool // EnableEnvelope generated the DEK in this process

	// store backs per-subject wrapped DEKs. EnableEnvelope retains it.
	store SecretStore

	mu       sync.Mutex
	subjects map[string]*SubjectVault // keyed by SubjectDEKSettingKey
}

// New derives a Cipher from the master secret. The DEK is loaded separately by
// EnableEnvelope once the settings store is available.
func New(secret string) *Cipher {
	c := &Cipher{
		// The KEK keeps the unlabelled SHA-256 derivation: the wrapped DEK
		// already in the settings table was sealed under it, so changing it
		// would make saved data unreadable. Every other use of the master
		// secret derives a labelled subkey (DeriveKey), which leaves this
		// derivation as the KEK's only consumer.
		kek: sha256.Sum256([]byte(secret)),
	}
	copy(c.mac[:], DeriveKey([]byte(secret), PurposeCookieMAC))
	return c
}

// EnableEnvelope loads the wrapped DEK from the store (generating a fresh random
// one on first run). Idempotent.
//
// A missing DEK on a database that still holds ciphertext stops the boot: the
// operator restores the key or clears the values, and no stored value becomes
// unreadable behind their back.
func (c *Cipher) EnableEnvelope(store SecretStore) error {
	c.store = store
	if wrapped, ok := store.Get(DEKSettingKey); ok && wrapped != "" {
		dek, err := c.unwrapKey(wrapped)
		if err != nil {
			return fmt.Errorf("crypto: cannot unwrap DEK with OCTARQ_SECRET_KEY: %w", err)
		}
		c.dek = dek
		c.ready = true
		return nil
	}

	encrypted, err := store.HasEncryptedValues()
	if err != nil {
		return fmt.Errorf("crypto: read the encrypted values: %w", err)
	}
	if encrypted {
		return ErrDEKMissing
	}

	// First run: generate a fresh DEK and persist it wrapped under the KEK.
	if _, err := io.ReadFull(rand.Reader, c.dek[:]); err != nil {
		return fmt.Errorf("crypto: generate DEK: %w", err)
	}
	c.ready = true
	c.created = true
	w, err := sealWith(c.kek, c.dek[:], nil)
	if err != nil {
		return err
	}
	return store.Set(DEKSettingKey, w)
}

// DEKCreated reports whether EnableEnvelope generated the DEK in this process
// instead of loading one that was already stored. A caller uses this to ask the
// rest of the application whether anything still holds ciphertext the new key
// cannot read.
func (c *Cipher) DEKCreated() bool {
	return c.created
}

// unwrapKey opens a wrapped 32-byte key with the current KEK.
func (c *Cipher) unwrapKey(wrapped string) (dek [32]byte, err error) {
	if pt, e := openWith(c.kek, wrapped, nil); e == nil {
		if len(pt) != 32 {
			return dek, errors.New("crypto: wrapped DEK has wrong length")
		}
		copy(dek[:], pt)
		return dek, nil
	} else {
		err = e
	}
	return dek, err
}

// Encrypt seals plaintext at rest under the instance DEK and returns
// base64(nonce||ciphertext).
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	return c.EncryptWithAAD(plaintext, nil)
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	return c.DecryptWithAAD(encoded, nil)
}

// EncryptWithAAD seals plaintext under the instance DEK and binds aad to the
// ciphertext. A ciphertext sealed with one aad does not open with another.
func (c *Cipher) EncryptWithAAD(plaintext, aad []byte) (string, error) {
	if !c.ready {
		return "", errors.New("crypto: envelope not initialized")
	}
	return sealWith(c.dek, plaintext, aad)
}

// DecryptWithAAD reverses EncryptWithAAD. It fails when aad differs from the
// value used to seal the ciphertext.
func (c *Cipher) DecryptWithAAD(encoded string, aad []byte) ([]byte, error) {
	if !c.ready {
		return nil, errors.New("crypto: envelope not initialized")
	}
	return openWith(c.dek, encoded, aad)
}

// SubjectVault returns the vault whose DEK belongs to (ownerID, subject). The
// subject DEK is a fresh random key, wrapped under the same KEK and stored in
// the SecretStore. SubjectVault creates the key on the first call and caches
// the vault for the process.
//
// The vault never falls back to the instance DEK.
func (c *Cipher) SubjectVault(ownerID uint, subject string) (*SubjectVault, error) {
	if !c.ready {
		return nil, errors.New("crypto: envelope not initialized")
	}
	if c.store == nil {
		return nil, errors.New("crypto: subject vaults need a secret store")
	}
	key := SubjectDEKSettingKey(ownerID, subject)

	// The lock covers the store reads and writes. A concurrent shred must not
	// leave this call holding a key that the shred is deleting.
	c.mu.Lock()
	defer c.mu.Unlock()
	if v := c.subjects[key]; v != nil {
		return v, nil
	}

	var dek [32]byte
	wrapped, ok := c.store.Get(key)
	switch {
	case !ok:
		// A subject key is created on demand. Unlike the instance DEK, its
		// absence is the normal first call, not a lost key.
		if _, err := io.ReadFull(rand.Reader, dek[:]); err != nil {
			return nil, fmt.Errorf("crypto: generate subject DEK: %w", err)
		}
		w, err := sealWith(c.kek, dek[:], nil)
		if err != nil {
			return nil, err
		}
		if err := c.store.Set(key, w); err != nil {
			return nil, fmt.Errorf("crypto: save subject DEK: %w", err)
		}
	case wrapped == "":
		return nil, fmt.Errorf("crypto: subject DEK %q is empty", key)
	default:
		loaded, err := c.unwrapKey(wrapped)
		if err != nil {
			return nil, fmt.Errorf("crypto: cannot unwrap subject DEK %q: %w", key, err)
		}
		dek = loaded
	}

	if c.subjects == nil {
		c.subjects = make(map[string]*SubjectVault)
	}
	v := &SubjectVault{dek: dek, parent: c}
	c.subjects[key] = v
	return v, nil
}

// ShredSubjectKey destroys the subject's DEK. Data sealed under it becomes
// unreadable permanently. Other subjects and the instance DEK keep working.
//
// Shredding a subject that has no key returns nil: the goal state is reached.
func (c *Cipher) ShredSubjectKey(ownerID uint, subject string) error {
	if !c.ready {
		return errors.New("crypto: envelope not initialized")
	}
	if c.store == nil {
		return errors.New("crypto: subject vaults need a secret store")
	}
	key := SubjectDEKSettingKey(ownerID, subject)

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.store.Delete(key); err != nil {
		return fmt.Errorf("crypto: delete subject DEK: %w", err)
	}
	delete(c.subjects, key)
	return nil
}

// SubjectVault seals data under one subject's DEK. Encrypt and Decrypt never
// touch the instance DEK.
type SubjectVault struct {
	dek    [32]byte
	parent *Cipher // resolves further subject namespaces and shred requests
}

// Encrypt seals plaintext under the subject DEK.
func (v *SubjectVault) Encrypt(plaintext []byte) (string, error) {
	return v.EncryptWithAAD(plaintext, nil)
}

// Decrypt reverses Encrypt.
func (v *SubjectVault) Decrypt(encoded string) ([]byte, error) {
	return v.DecryptWithAAD(encoded, nil)
}

// EncryptWithAAD seals plaintext under the subject DEK and binds aad.
func (v *SubjectVault) EncryptWithAAD(plaintext, aad []byte) (string, error) {
	return sealWith(v.dek, plaintext, aad)
}

// DecryptWithAAD reverses EncryptWithAAD.
func (v *SubjectVault) DecryptWithAAD(encoded string, aad []byte) ([]byte, error) {
	return openWith(v.dek, encoded, aad)
}

// SubjectVault returns the vault of (ownerID, subject). Subject vaults share
// the instance KEK and key store, so the call resolves the same namespace as
// the root vault.
func (v *SubjectVault) SubjectVault(ownerID uint, subject string) (*SubjectVault, error) {
	return v.parent.SubjectVault(ownerID, subject)
}

// ShredSubjectKey destroys the subject's DEK. See Cipher.ShredSubjectKey.
func (v *SubjectVault) ShredSubjectKey(ownerID uint, subject string) error {
	return v.parent.ShredSubjectKey(ownerID, subject)
}

// sealWith encrypts plaintext under key and returns
// base64(nonce||ciphertext||tag), with aad bound to the ciphertext.
func sealWith(key [32]byte, plaintext, aad []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plaintext, aad)), nil
}

// openWith decrypts base64(nonce||ciphertext) under key and checks aad.
func openWith(key [32]byte, encoded string, aad []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, aad)
}

func newGCM(key [32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Sign returns base64(HMAC-SHA256(msg)) under the cookie-MAC subkey, not the
// KEK: a signature oracle must never expose the key that wraps the DEK.
func (c *Cipher) Sign(msg []byte) string {
	mac := hmac.New(sha256.New, c.mac[:])
	mac.Write(msg)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify checks an HMAC produced by Sign in constant time.
func (c *Cipher) Verify(msg []byte, sig string) bool {
	return hmac.Equal([]byte(c.Sign(msg)), []byte(sig))
}
