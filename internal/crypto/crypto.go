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
	"errors"
	"fmt"
	"io"
)

// dekSettingKey is where the wrapped DEK lives in the settings table.
const dekSettingKey = "crypto.dek"

// SecretStore is the minimal key/value access the envelope bootstrap needs to
// persist the wrapped DEK. The app backs it with the settings table; keeping it
// an interface avoids a gorm dependency here.
type SecretStore interface {
	Get(key string) (string, bool)
	Set(key, val string) error
}

// Cipher encrypts secrets at rest (under the DEK) and signs session cookies
// (under the KEK). The KEK is derived from OCTARQ_SECRET_KEY; the DEK is loaded by
// EnableEnvelope. Until then only Sign/Verify (which use the KEK) are usable.
type Cipher struct {
	kek   [32]byte
	dek   [32]byte
	ready bool // DEK loaded (EnableEnvelope ran)
}

// New derives a Cipher from the master secret. The DEK is loaded separately by
// EnableEnvelope once the settings store is available.
func New(secret string) *Cipher {
	return &Cipher{kek: sha256.Sum256([]byte(secret))}
}

// EnableEnvelope loads the wrapped DEK from the store (generating a fresh random
// one on first run). Idempotent.
func (c *Cipher) EnableEnvelope(store SecretStore) error {
	if wrapped, ok := store.Get(dekSettingKey); ok && wrapped != "" {
		dek, err := c.unwrapDEK(wrapped)
		if err != nil {
			return fmt.Errorf("crypto: cannot unwrap DEK with OCTARQ_SECRET_KEY: %w", err)
		}
		c.dek = dek
		c.ready = true
		return nil
	}

	// First run: generate a fresh DEK and persist it wrapped under the KEK.
	if _, err := io.ReadFull(rand.Reader, c.dek[:]); err != nil {
		return fmt.Errorf("crypto: generate DEK: %w", err)
	}
	c.ready = true
	w, err := sealWith(c.kek, c.dek[:])
	if err != nil {
		return err
	}
	return store.Set(dekSettingKey, w)
}

// unwrapDEK opens the wrapped DEK with the current KEK.
func (c *Cipher) unwrapDEK(wrapped string) (dek [32]byte, err error) {
	if pt, e := openWith(c.kek, wrapped); e == nil {
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

// Encrypt seals plaintext at rest under the DEK and returns
// base64(nonce||ciphertext).
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	if !c.ready {
		return "", errors.New("crypto: envelope not initialized")
	}
	return sealWith(c.dek, plaintext)
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	if !c.ready {
		return nil, errors.New("crypto: envelope not initialized")
	}
	return openWith(c.dek, encoded)
}

// sealWith encrypts plaintext under key and returns base64(nonce||ciphertext).
func sealWith(key [32]byte, plaintext []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plaintext, nil)), nil
}

// openWith decrypts base64(nonce||ciphertext) under key.
func openWith(key [32]byte, encoded string) ([]byte, error) {
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
	return gcm.Open(nil, nonce, ct, nil)
}

func newGCM(key [32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Sign returns base64(HMAC-SHA256(msg)) under the KEK (the master key, not the
// DEK), so cookie signing is independent of data-at-rest.
func (c *Cipher) Sign(msg []byte) string {
	mac := hmac.New(sha256.New, c.kek[:])
	mac.Write(msg)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify checks an HMAC produced by Sign in constant time.
func (c *Cipher) Verify(msg []byte, sig string) bool {
	return hmac.Equal([]byte(c.Sign(msg)), []byte(sig))
}
