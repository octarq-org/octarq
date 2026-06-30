// Package crypto provides AES-GCM encryption for secrets at rest and HMAC
// signing for session cookies.
//
// # Envelope encryption (key rotation)
//
// Secrets at rest are encrypted under a random Data Encryption Key (DEK). The
// DEK itself is wrapped (encrypted) under a Key Encryption Key (KEK) derived
// from LED_SECRET_KEY and stored in the settings table. Rotating LED_SECRET_KEY
// therefore only re-wraps the one DEK — the bulk data is never touched. This is
// the standard KMS/Vault envelope pattern.
//
// Rotation: start once with LED_SECRET_KEY=new and LED_SECRET_KEY_OLD=old. On
// boot the DEK is unwrapped with the old key and re-wrapped under the new one
// (saved), after which LED_SECRET_KEY_OLD can be dropped. Cookies signed under
// the old key become invalid on rotation, so sessions simply re-login.
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
// (under the KEK). The KEK is derived from LED_SECRET_KEY; the DEK is loaded by
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
// one on first run). For a key rotation, pass the previous master secret(s) in
// rotateFrom: the DEK is unwrapped with whichever key works and then re-wrapped
// under the current KEK and saved, so the next restart needs only the current
// LED_SECRET_KEY. Idempotent.
func (c *Cipher) EnableEnvelope(store SecretStore, rotateFrom ...string) error {
	if wrapped, ok := store.Get(dekSettingKey); ok && wrapped != "" {
		dek, rewrapped, err := c.unwrapDEK(wrapped, rotateFrom)
		if err != nil {
			return fmt.Errorf("crypto: cannot unwrap DEK with LED_SECRET_KEY (rotating? set LED_SECRET_KEY_OLD): %w", err)
		}
		c.dek = dek
		c.ready = true
		if rewrapped {
			if w, err := sealWith(c.kek, c.dek[:]); err == nil {
				_ = store.Set(dekSettingKey, w)
			}
		}
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

// unwrapDEK opens the wrapped DEK with the current KEK, falling back to each
// rotateFrom key. rewrapped is true when an old key opened it (caller should
// re-wrap under the current KEK).
func (c *Cipher) unwrapDEK(wrapped string, rotateFrom []string) (dek [32]byte, rewrapped bool, err error) {
	if pt, e := openWith(c.kek, wrapped); e == nil {
		if len(pt) != 32 {
			return dek, false, errors.New("crypto: wrapped DEK has wrong length")
		}
		copy(dek[:], pt)
		return dek, false, nil
	} else {
		err = e
	}
	for _, old := range rotateFrom {
		if old == "" {
			continue
		}
		oldKEK := sha256.Sum256([]byte(old))
		if pt, e := openWith(oldKEK, wrapped); e == nil {
			if len(pt) != 32 {
				return dek, false, errors.New("crypto: wrapped DEK has wrong length")
			}
			copy(dek[:], pt)
			return dek, true, nil
		}
	}
	return dek, false, err
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
