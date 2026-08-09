// Package csrf mints the double-submit token that pairs with the session
// cookie. The token is DERIVED from the session token rather than stored, so
// there is no column to migrate, no row to look up on the hot path, and no way
// for the two to fall out of step: a rotated session yields a new token and
// invalidates the old one for free.
package csrf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"
)

// CookieName is the readable (non-HttpOnly) half of the double submit. The
// browser must be able to read it to echo the value back in HeaderName, which
// is exactly what an attacker's page on another origin cannot do.
const CookieName = "octarq_csrf"

// HeaderName carries the echoed token on state-changing requests.
const HeaderName = "X-CSRF-Token"

// CookieTTL mirrors auth.sessionTTL. The token is meaningless once the session
// it was derived from is gone, so the two lifetimes are kept equal — pinned by
// TestCSRFCookieTTLMatchesSessionTTL in internal/auth, since Go's import
// direction won't let this package read the session TTL directly.
const CookieTTL = 7 * 24 * time.Hour

// GenerateToken derives a double-submit CSRF token from a session token using
// HMAC-SHA256 keyed by the instance secret. Compare results with hmac.Equal,
// never ==.
func GenerateToken(secretKey, sessionToken string) string {
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(sessionToken))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// NewCookie builds the readable token cookie for a session token. secure must
// be derived the same way the session cookie's own Secure attribute is, or the
// pair splits across HTTP/HTTPS and every write starts failing.
func NewCookie(secretKey, sessionToken string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    GenerateToken(secretKey, sessionToken),
		Path:     "/",
		HttpOnly: false, // deliberate: the frontend has to read it
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(CookieTTL.Seconds()),
	}
}

// ClearCookie expires the token cookie; pair it with clearing the session.
func ClearCookie() *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1}
}
