package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
)

// The challenge signature must use the challenge subkey, not the raw master
// secret the session cookies and CSRF tokens also use.
func TestTwoFAChallengeUsesItsOwnSubkey(t *testing.T) {
	const secret = "master-secret"
	m := New(&config.Config{SecretKey: secret}, crypto.New(secret))

	tok, err := m.mintChallenge(7, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("mintChallenge: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		t.Fatalf("want payload.signature, got %q", tok)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	mac := hmac.New(sha256.New, crypto.DeriveKey([]byte(secret), "octarq/twofa-challenge/v1"))
	mac.Write(payload)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[1] != want {
		t.Fatalf("challenge must be signed with the challenge subkey: got %q want %q", parts[1], want)
	}

	raw := hmac.New(sha256.New, []byte(secret))
	raw.Write(payload)
	if parts[1] == base64.RawURLEncoding.EncodeToString(raw.Sum(nil)) {
		t.Fatal("challenge must not be signed with the raw master secret")
	}
}

// The OAuth state cookie is signed and encrypted by the goth session store, so
// its key must be the OAuth subkey, not the raw master secret.
func TestOAuthStateKeyUsesItsOwnSubkey(t *testing.T) {
	const secret = "master-secret"
	if got, want := oauthStateKey(secret), crypto.DeriveKey([]byte(secret), "octarq/oauth-state/v1"); string(got) != string(want) {
		t.Fatalf("OAuth state key must be the oauth-state subkey")
	}
	if string(oauthStateKey(secret)) == secret {
		t.Fatal("OAuth state key must not be the raw master secret")
	}
}
