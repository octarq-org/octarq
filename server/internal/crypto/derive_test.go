package crypto

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

// Every purpose gets its own key. Two jobs sharing key material is what the
// purpose labels exist to prevent, and a derivation that ignores its label
// would make every caller's "different key" assumption false.
func TestDeriveKeyIsPerPurpose(t *testing.T) {
	secret := []byte("master-secret")

	first := DeriveKey(secret, "octarq/cookie-mac/v1")
	if len(first) != 32 {
		t.Fatalf("want a 32-byte key, got %d", len(first))
	}
	if !bytes.Equal(first, DeriveKey(secret, "octarq/cookie-mac/v1")) {
		t.Fatal("the same secret and purpose must derive the same key")
	}
	if bytes.Equal(first, DeriveKey(secret, "octarq/csrf/v1")) {
		t.Fatal("two purposes must not share a key")
	}
	if bytes.Equal(first, DeriveKey([]byte("other-secret"), "octarq/cookie-mac/v1")) {
		t.Fatal("two secrets must not derive the same purpose key")
	}

	// The KEK keeps its unlabelled SHA-256 derivation, so a labelled key that
	// equalled it would put the DEK-wrapping key back on the signing path.
	kek := sha256.Sum256(secret)
	if bytes.Equal(first, kek[:]) {
		t.Fatal("a purpose key must not equal the KEK derivation")
	}
}

// Sign must produce an HMAC under the cookie-MAC subkey. Pinning the exact key
// catches a signer that silently falls back to the KEK or the raw secret.
func TestCipherSignUsesTheCookieMACSubkey(t *testing.T) {
	const secret = "master-secret"
	msg := []byte("session-token")

	c := New(secret)
	mac := hmac.New(sha256.New, DeriveKey([]byte(secret), "octarq/cookie-mac/v1"))
	mac.Write(msg)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if got := c.Sign(msg); got != want {
		t.Fatalf("Sign must use the cookie-MAC subkey: got %q want %q", got, want)
	}

	raw := hmac.New(sha256.New, []byte(secret))
	raw.Write(msg)
	if got := base64.RawURLEncoding.EncodeToString(raw.Sum(nil)); c.Sign(msg) == got {
		t.Fatal("Sign must not use the raw master secret as the MAC key")
	}
}
