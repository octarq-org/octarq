package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

// twofaChallengeTTL bounds how long a login may sit between the proof that
// started it — an OAuth provider round-trip — and the second factor that
// completes it. Short enough that a captured challenge is not a standing
// backdoor, long enough that a human can fetch their authenticator app.
const twofaChallengeTTL = 10 * time.Minute

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
