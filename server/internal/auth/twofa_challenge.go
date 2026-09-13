package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/octarq/origin"
)

// twofaChallengeTTL bounds how long a login may sit between the proof that
// started it — an OAuth provider round-trip — and the second factor that
// completes it. Short enough that a captured challenge is not a standing
// backdoor, long enough that a human can fetch their authenticator app.
const twofaChallengeTTL = 10 * time.Minute

// twofaChallengeCookie is the HttpOnly cookie carrying the pending challenge
// between the OAuth callback and /api/auth/2fa/verify. Path is narrowed to the
// single endpoint that consumes it: the 2FA page itself only needs the
// ?twofa=1 fact in its URL, never the challenge value, so the cookie has no
// business on /admin/ or any other /api/ route.
const twofaChallengeCookie = "octarq_2fa_challenge"

// twofaChallengePath is the one path the challenge cookie must reach. Set and
// clear must agree on it or the browser would treat them as two different
// cookies (one for the URL, one for the jar's clearing rules).
const twofaChallengePath = "/api/auth/2fa/verify"

// NewTwoFAChallenge mints a short-lived, signed challenge token that proves
// the holder has just completed an OAuth login round-trip as uid. It is the
// OAuth analogue of "the client re-sends email+password" in verify2FA: the
// browser cannot re-send a password the OAuth account never had, so the
// signed challenge stands in for it when the second-factor step completes
// (see api.verify2FA's challengeToken branch).
//
// The token is bound to the user by signature, not by a stored row: there is
// deliberately no second pending-state table to drift from the password path.
// Expiry is inside the signed payload, so a token cannot be extended by
// editing the cookie or the URL. It is still not a session — completing 2FA
// is what mints the session.
func (m *Manager) NewTwoFAChallenge(uid uint) (string, error) {
	return m.mintChallenge(uid, time.Now().Add(twofaChallengeTTL))
}

func (m *Manager) mintChallenge(uid uint, expiry time.Time) (string, error) {
	if m.cfg.SecretKey == "" {
		return "", errors.New("auth: no secret key configured")
	}
	// uid (8) + expiry (8) + nonce (16): the nonce keeps two challenges for
	// the same user in the same second from being byte-identical.
	payload := make([]byte, 8+8+16)
	binary.BigEndian.PutUint64(payload[0:8], uint64(uid))
	binary.BigEndian.PutUint64(payload[8:16], uint64(expiry.Unix()))
	if _, err := rand.Read(payload[16:]); err != nil {
		return "", err
	}
	sig := hmac.New(sha256.New, []byte(m.cfg.SecretKey))
	sig.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(sig.Sum(nil)), nil
}

// SetTwoFAChallengeCookie hands the pending OAuth 2FA challenge to the browser
// in an HttpOnly cookie instead of a URL query parameter, so it never lands in
// proxy access logs or browser history. The attributes mirror the session
// cookie (auth.go setCookie): Secure derived per request via origin.Secure and
// the same trustProxy gate, SameSite=Lax, HttpOnly, and Max-Age equal to the
// challenge's 10-minute TTL so the cookie and the credential expire together.
func (m *Manager) SetTwoFAChallengeCookie(w http.ResponseWriter, r *http.Request, challenge string) {
	http.SetCookie(w, &http.Cookie{
		Name:     twofaChallengeCookie,
		Value:    challenge,
		Path:     twofaChallengePath,
		HttpOnly: true,
		Secure:   origin.Secure(r, trustProxy),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(twofaChallengeTTL.Seconds()),
	})
}

// TwoFAChallengeFromRequest reads the pending challenge cookie, or "" when the
// request carries none.
func (m *Manager) TwoFAChallengeFromRequest(r *http.Request) string {
	c, err := r.Cookie(twofaChallengeCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// ClearTwoFAChallengeCookie expires the challenge cookie. Called the moment a
// session is issued for a challenge (spent) and when a challenge is refused as
// invalid (dead) — never on a wrong TOTP code, where the challenge is still
// good and the user gets to retry.
func (m *Manager) ClearTwoFAChallengeCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     twofaChallengeCookie,
		Value:    "",
		Path:     twofaChallengePath,
		HttpOnly: true,
		Secure:   origin.Secure(r, trustProxy),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// VerifyTwoFAChallenge validates a challenge token's signature and expiry and
// returns the user ID it names, or 0 when the token is forged, expired or
// malformed. It is the only consumer of the twofaChallengeTTL: a challenge
// that outlived its window is refused rather than refreshed.
func (m *Manager) VerifyTwoFAChallenge(token string) uint {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return 0
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) != 8+8+16 {
		return 0
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0
	}
	want := hmac.New(sha256.New, []byte(m.cfg.SecretKey))
	want.Write(payload)
	if !hmac.Equal(sig, want.Sum(nil)) {
		return 0
	}
	if expiry := binary.BigEndian.Uint64(payload[8:16]); expiry < uint64(time.Now().Unix()) {
		return 0
	}
	return uint(binary.BigEndian.Uint64(payload[0:8]))
}
