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
	"sync/atomic"
)

const DEKSettingKey = "crypto.dek"

const SubjectDEKKeyPrefix = "crypto.dek.subject."

func SubjectDEKSettingKey(ownerID uint, subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return fmt.Sprintf("%s%d.%s", SubjectDEKKeyPrefix, ownerID, hex.EncodeToString(sum[:])[:32])
}

type SecretStore interface {
	Get(key string) (string, bool)
	Set(key, val string) error
	Delete(key string) error
	HasEncryptedValues() (bool, error)
}

var ErrDEKMissing = errors.New("crypto: the key that decrypts saved values is missing. Restore the database backup taken before the upgrade, or set the affected values again: notification channels, webhook secrets, two-factor authentication, and OAuth client secrets")

type Cipher struct {
	kek     [32]byte
	mac     [32]byte
	dek     [32]byte
	ready   bool
	created bool

	store SecretStore

	mu       sync.Mutex
	subjects map[string]*SubjectVault
}

func New(secret string) *Cipher {
	c := &Cipher{
		kek: sha256.Sum256([]byte(secret)),
	}
	copy(c.mac[:], DeriveKey([]byte(secret), PurposeCookieMAC))
	return c
}

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

func (c *Cipher) DEKCreated() bool {
	return c.created
}

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

func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	return c.EncryptWithAAD(plaintext, nil)
}

func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	return c.DecryptWithAAD(encoded, nil)
}

func (c *Cipher) EncryptWithAAD(plaintext, aad []byte) (string, error) {
	if !c.ready {
		return "", errors.New("crypto: envelope not initialized")
	}
	return sealWith(c.dek, plaintext, aad)
}

func (c *Cipher) DecryptWithAAD(encoded string, aad []byte) ([]byte, error) {
	if !c.ready {
		return nil, errors.New("crypto: envelope not initialized")
	}
	return openWith(c.dek, encoded, aad)
}

func (c *Cipher) HasSubjectKey(ownerID uint, subject string) bool {
	if c == nil || !c.ready || c.store == nil {
		return false
	}
	key := SubjectDEKSettingKey(ownerID, subject)
	wrapped, ok := c.store.Get(key)
	return ok && wrapped != ""
}

func (c *Cipher) SubjectVault(ownerID uint, subject string) (*SubjectVault, error) {
	if !c.ready {
		return nil, errors.New("crypto: envelope not initialized")
	}
	if c.store == nil {
		return nil, errors.New("crypto: subject vaults need a secret store")
	}
	key := SubjectDEKSettingKey(ownerID, subject)

	c.mu.Lock()
	defer c.mu.Unlock()
	if v := c.subjects[key]; v != nil {
		return v, nil
	}

	var dek [32]byte
	wrapped, ok := c.store.Get(key)
	switch {
	case !ok:
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
	if v := c.subjects[key]; v != nil {
		v.dead.Store(true)
	}
	delete(c.subjects, key)
	return nil
}

type SubjectVault struct {
	dek    [32]byte
	parent *Cipher
	dead   atomic.Bool
}

var errSubjectShredded = errors.New("crypto: the subject key was shredded")

func (v *SubjectVault) Encrypt(plaintext []byte) (string, error) {
	return v.EncryptWithAAD(plaintext, nil)
}

func (v *SubjectVault) Decrypt(encoded string) ([]byte, error) {
	return v.DecryptWithAAD(encoded, nil)
}

func (v *SubjectVault) EncryptWithAAD(plaintext, aad []byte) (string, error) {
	if v.dead.Load() {
		return "", errSubjectShredded
	}
	return sealWith(v.dek, plaintext, aad)
}

func (v *SubjectVault) DecryptWithAAD(encoded string, aad []byte) ([]byte, error) {
	if v.dead.Load() {
		return nil, errSubjectShredded
	}
	return openWith(v.dek, encoded, aad)
}

func (v *SubjectVault) SubjectVault(ownerID uint, subject string) (*SubjectVault, error) {
	return v.parent.SubjectVault(ownerID, subject)
}

func (v *SubjectVault) ShredSubjectKey(ownerID uint, subject string) error {
	return v.parent.ShredSubjectKey(ownerID, subject)
}

func (v *SubjectVault) HasSubjectKey(ownerID uint, subject string) bool {
	return v.parent.HasSubjectKey(ownerID, subject)
}

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

func (c *Cipher) Sign(msg []byte) string {
	mac := hmac.New(sha256.New, c.mac[:])
	mac.Write(msg)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (c *Cipher) Verify(msg []byte, sig string) bool {
	return hmac.Equal([]byte(c.Sign(msg)), []byte(sig))
}
