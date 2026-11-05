package csrf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/crypto"
)

// TestGenerateTokenDerivesDeterministicHMAC pins the derived token: same inputs
// yield the same value, a different session token or secret changes it, and the
// result round-trips through hmac.Equal (not ==) client code.
func TestGenerateTokenDerivesDeterministicHMAC(t *testing.T) {
	a := GenerateToken("secret", "session-1")
	if a == "" {
		t.Fatal("empty token")
	}
	if b := GenerateToken("secret", "session-1"); b != a {
		t.Errorf("same inputs produced different tokens: %q vs %q", a, b)
	}
	if b := GenerateToken("secret", "session-2"); b == a {
		t.Error("different session tokens produced the same token")
	}
	if b := GenerateToken("other-secret", "session-1"); b == a {
		t.Error("different secrets produced the same token")
	}
}

// TestNewCookieAttributes pins the double-submit cookie shape: readable
// (non-HttpOnly), same-site lax, session-length TTL, and the Secure flag passed
// through by the caller (it must mirror the session cookie's).
func TestNewCookieAttributes(t *testing.T) {
	c := NewCookie("secret", "session-x", true)
	if c.Name != CookieName {
		t.Errorf("name = %q, want %q", c.Name, CookieName)
	}
	if c.HttpOnly {
		t.Error("token cookie must be readable by the frontend (HttpOnly=false)")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if !c.Secure {
		t.Error("Secure flag not propagated")
	}
	if c.MaxAge != int(CookieTTL.Seconds()) {
		t.Errorf("MaxAge = %d, want session TTL", c.MaxAge)
	}
	if c.Value != GenerateToken("secret", "session-x") {
		t.Error("cookie value is not the derived token")
	}

	insecure := NewCookie("secret", "session-x", false)
	if insecure.Secure {
		t.Error("Secure flag forced on for an insecure request")
	}
}

// TestClearCookieExpires pins the logout cookie: empty value, negative MaxAge.
func TestClearCookieExpires(t *testing.T) {
	c := ClearCookie()
	if c.Name != CookieName || c.Value != "" || c.MaxAge != -1 {
		t.Errorf("ClearCookie = %+v", c)
	}
}

// The CSRF token must be keyed by the CSRF subkey, not by the master secret:
// the same secret also signs session cookies and the second-factor challenge,
// and a token verifier in one use must not accept a signature from another.
func TestGenerateTokenUsesTheCSRFSubkey(t *testing.T) {
	const secret = "master-secret"
	const session = "session-token"

	derived := hmac.New(sha256.New, crypto.DeriveKey([]byte(secret), "octarq/csrf/v1"))
	derived.Write([]byte(session))
	want := base64.RawURLEncoding.EncodeToString(derived.Sum(nil))
	if got := GenerateToken(secret, session); got != want {
		t.Fatalf("token must use the CSRF subkey: got %q want %q", got, want)
	}

	raw := hmac.New(sha256.New, []byte(secret))
	raw.Write([]byte(session))
	if GenerateToken(secret, session) == base64.RawURLEncoding.EncodeToString(raw.Sum(nil)) {
		t.Fatal("token must not be keyed by the raw master secret")
	}
}
