package csrf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/server/internal/crypto"
)

const CookieName = "octarq_csrf"

const HeaderName = "X-CSRF-Token"

const CookieTTL = 7 * 24 * time.Hour

func GenerateToken(secretKey, sessionToken string) string {
	h := hmac.New(sha256.New, crypto.DeriveKey([]byte(secretKey), crypto.PurposeCSRF))
	h.Write([]byte(sessionToken))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func NewCookie(secretKey, sessionToken string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    GenerateToken(secretKey, sessionToken),
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(CookieTTL.Seconds()),
	}
}

func ClearCookie() *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1}
}
