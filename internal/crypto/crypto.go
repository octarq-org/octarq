// Package crypto provides AES-GCM encryption for secrets at rest and HMAC
// signing for session cookies. The key is derived from the configured secret.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// Cipher wraps a key derived from the app secret.
type Cipher struct {
	key [32]byte
}

// New derives a 256-bit key from the given secret via SHA-256.
func New(secret string) *Cipher {
	return &Cipher{key: sha256.Sum256([]byte(secret))}
}

// Encrypt seals plaintext with AES-256-GCM and returns base64(nonce||ciphertext).
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// Sign returns base64(HMAC-SHA256(msg)).
func (c *Cipher) Sign(msg []byte) string {
	mac := hmac.New(sha256.New, c.key[:])
	mac.Write(msg)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify checks an HMAC produced by Sign in constant time.
func (c *Cipher) Verify(msg []byte, sig string) bool {
	expected := c.Sign(msg)
	return hmac.Equal([]byte(expected), []byte(sig))
}
