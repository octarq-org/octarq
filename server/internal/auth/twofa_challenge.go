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

	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/origin"
)

const twofaChallengeTTL = 10 * time.Minute

const twofaChallengeCookie = "octarq_2fa_challenge"

const twofaChallengePath = "/api/auth/2fa/verify"

func (m *Manager) NewTwoFAChallenge(uid uint) (string, error) {
	return m.mintChallenge(uid, time.Now().Add(twofaChallengeTTL))
}

func (m *Manager) mintChallenge(uid uint, expiry time.Time) (string, error) {
	if m.cfg.SecretKey == "" {
		return "", errors.New("auth: no secret key configured")
	}
	payload := make([]byte, 8+8+16)
	binary.BigEndian.PutUint64(payload[0:8], uint64(uid))
	binary.BigEndian.PutUint64(payload[8:16], uint64(expiry.Unix()))
	if _, err := rand.Read(payload[16:]); err != nil {
		return "", err
	}
	sig := hmac.New(sha256.New, crypto.DeriveKey([]byte(m.cfg.SecretKey), crypto.PurposeTwoFAChallenge))
	sig.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(sig.Sum(nil)), nil
}

func (m *Manager) SetTwoFAChallengeCookie(w http.ResponseWriter, r *http.Request, challenge string) {
	http.SetCookie(w, &http.Cookie{
		Name:     twofaChallengeCookie,
		Value:    challenge,
		Path:     twofaChallengePath,
		HttpOnly: true,
		Secure:   origin.Secure(r, trustProxy.Load()),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(twofaChallengeTTL.Seconds()),
	})
}

func (m *Manager) TwoFAChallengeFromRequest(r *http.Request) string {
	c, err := r.Cookie(twofaChallengeCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func (m *Manager) ClearTwoFAChallengeCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     twofaChallengeCookie,
		Value:    "",
		Path:     twofaChallengePath,
		HttpOnly: true,
		Secure:   origin.Secure(r, trustProxy.Load()),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

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
	want := hmac.New(sha256.New, crypto.DeriveKey([]byte(m.cfg.SecretKey), crypto.PurposeTwoFAChallenge))
	want.Write(payload)
	if !hmac.Equal(sig, want.Sum(nil)) {
		return 0
	}
	if expiry := binary.BigEndian.Uint64(payload[8:16]); expiry < uint64(time.Now().Unix()) {
		return 0
	}
	return uint(binary.BigEndian.Uint64(payload[0:8]))
}
