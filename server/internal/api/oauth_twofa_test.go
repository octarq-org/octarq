package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/markbates/goth/gothic"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
)

// roundTripFunc adapts a closure to http.RoundTripper so goth's google
// provider — which fetches the profile over http.DefaultClient — can be
// answered with a fixed profile instead of a live IdP.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// oauthTestEnv wires what an /auth/callback/{provider} round-trip needs in a
// test handler: Google OAuth settings, a registered domain (so the callback
// resolves an origin), a stubbed provider profile fetch, and a pre-completed
// goth session under the name the handler will look it up by. It returns the
// goth session cookies to carry on the callback request.
func oauthTestEnv(t *testing.T, h *Handler, db *gorm.DB, email string) []*http.Cookie {
	t.Helper()
	auth.InitGothStore("secret")

	db.Where("key LIKE ?", "oauth.%").Delete(&models.Setting{})
	enc, err := h.cipher.Encrypt([]byte("google-secret"))
	if err != nil {
		t.Fatalf("encrypt google secret: %v", err)
	}
	db.Create(&models.Setting{Key: "oauth.google.client_id", Value: "google-id"})
	db.Create(&models.Setting{Key: "oauth.google.client_secret", Value: string(enc)})
	if err := db.Create(&dns.Domain{OrgID: 1, Name: "acme.example"}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "userinfo") {
			t.Fatalf("unexpected provider fetch: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"email":"` + email + `","name":"Test"}`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	// The callback resolves origin from the registered domain, so the goth
	// provider name is "<provider>@https://app.acme.example" (see
	// OAuthHandler.prepare). Seed the finished OAuth round-trip under it.
	// AuthURL must be set — gothic's state validation refuses a session whose
	// AuthURL is empty — and carries no state, so no ?state= is required.
	const gothName = "google@https://app.acme.example"
	const gothSessionJSON = `{"AccessToken":"test-access-token","AuthURL":"https://accounts.google.com/o/oauth2/auth"}`
	seedRec := httptest.NewRecorder()
	seedReq := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := gothic.StoreInSession(gothName, gothSessionJSON, seedReq, seedRec); err != nil {
		t.Fatalf("store goth session: %v", err)
	}
	return seedRec.Result().Cookies()
}

// oauthCallbackAs runs the real /auth/callback/google route carrying the
// seeded goth session, and returns the response.
func oauthCallbackAs(t *testing.T, srv http.Handler, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/auth/callback/google", nil)
	req.Host = "app.acme.example"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == "octarq_session" {
			return c
		}
	}
	return nil
}

func challengeCookie(cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == "octarq_2fa_challenge" {
			return c
		}
	}
	return nil
}

// Guard test 1 (core): a TOTP-enabled account arriving through the OAuth
// callback must NOT receive a session. The callback redirects to the same 2FA
// page password login uses, storing the short-lived challenge in an HttpOnly
// cookie; only /api/auth/2fa/verify with the code mints the session.
func TestOAuthCallbackRequiresTwoFactorForTOTPUsers(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	disableEmailVerification(t, db)

	// Register bob and enroll TOTP through the real API.
	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"bob@example.com","password":"secret-pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	regCookies := rec.Result().Cookies()
	rec = do(srv, "POST", "/api/auth/2fa/setup", regCookies, `{"password":"secret-pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa setup: got %d (%s)", rec.Code, rec.Body.String())
	}
	var setup struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &setup); err != nil || setup.Secret == "" {
		t.Fatalf("setup response: %s", rec.Body.String())
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/enable", regCookies, `{"code":"`+code+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa enable: got %d (%s)", rec.Code, rec.Body.String())
	}

	cb := oauthCallbackAs(t, srv, oauthTestEnv(t, h, db, "bob@example.com"))

	if cb.Code != http.StatusFound {
		t.Fatalf("oauth callback: got %d (%s), want a redirect to the 2FA page", cb.Code, cb.Body.String())
	}
	loc := cb.Header().Get("Location")
	if !strings.Contains(loc, "twofa=1") {
		t.Fatalf("callback did not send the browser to the 2FA page: %q", loc)
	}
	if sess := sessionCookie(cb.Result().Cookies()); sess != nil {
		t.Fatal("OAuth callback set a session cookie while 2FA is pending")
	}

	// The challenge arrives in an HttpOnly cookie with a Max-Age matching the
	// 10-minute TTL, not in the redirect URL.
	challenge := challengeCookie(cb.Result().Cookies())
	if challenge == nil || !challenge.HttpOnly {
		t.Fatalf("OAuth callback set no 2FA challenge cookie, or it is not HttpOnly: %+v", challenge)
	}
	if challenge.MaxAge != int((10 * time.Minute).Seconds()) {
		t.Fatalf("challenge cookie MaxAge = %d, want %d (10-minute TTL)", challenge.MaxAge, int((10 * time.Minute).Seconds()))
	}
	// Guard (this fix): the challenge value must never appear in the redirect
	// Location — the URL is what proxy access logs and browser history record.
	if strings.Contains(loc, challenge.Value) {
		t.Fatalf("challenge value leaked into the redirect Location %q", loc)
	}

	// The verify endpoint reads the challenge from the cookie: with the cookie
	// but no code the login must fail without issuing a session.
	rec = do(srv, "POST", "/api/auth/2fa/verify", []*http.Cookie{challenge}, `{"code":"000000"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("challenge without code: got %d, want 401", rec.Code)
	}
	if sess := sessionCookie(rec.Result().Cookies()); sess != nil {
		t.Fatal("challenge without code set a session cookie")
	}
	// A wrong code is retryable: the challenge cookie must NOT be cleared, or a
	// typo would force a full OAuth restart.
	if c := challengeCookie(rec.Result().Cookies()); c != nil {
		t.Fatalf("wrong code cleared the challenge cookie (MaxAge=%d), want it kept for retry", c.MaxAge)
	}

	// Guard: the challenge is read from the cookie — a request carrying only
	// the code, no challenge cookie, cannot complete an OAuth login.
	code, _ = totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil, `{"code":"`+code+`"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify without challenge cookie: got %d, want 401", rec.Code)
	}

	// The shared verify2FA endpoint completes the login: challenge cookie +
	// TOTP code mints a usable session and spends the challenge.
	code, _ = totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", []*http.Cookie{challenge}, `{"code":"`+code+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa verify via challenge: got %d (%s)", rec.Code, rec.Body.String())
	}
	sess := sessionCookie(rec.Result().Cookies())
	if sess == nil {
		t.Fatal("2fa verify via challenge set no session cookie")
		return
	}
	// Guard: the spent challenge cookie is cleared in the same response.
	c := challengeCookie(rec.Result().Cookies())
	if c == nil || c.MaxAge >= 0 {
		t.Fatalf("spent challenge cookie was not cleared after successful verify: got %+v", c)
	}
	if rec := do(srv, "GET", "/api/overview", []*http.Cookie{sess}, ""); rec.Code != http.StatusOK {
		t.Fatalf("session from challenge+code is not usable: got %d", rec.Code)
	}
}

// Guard test 2: an account without TOTP keeps the pre-fix behaviour — the
// OAuth callback signs it straight in.
func TestOAuthCallbackWithoutTwoFactorStillSignsIn(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	disableEmailVerification(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"carol@example.com","password":"secret-pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}

	cb := oauthCallbackAs(t, srv, oauthTestEnv(t, h, db, "carol@example.com"))

	if cb.Code != http.StatusFound {
		t.Fatalf("oauth callback: got %d (%s), want 302", cb.Code, cb.Body.String())
	}
	if loc := cb.Header().Get("Location"); strings.Contains(loc, "twofa=") {
		t.Fatalf("non-TOTP user was sent to the 2FA page: %q", loc)
	}
	sess := sessionCookie(cb.Result().Cookies())
	if sess == nil {
		t.Fatal("OAuth callback set no session cookie for a non-TOTP user")
		return
	}
	if rec := do(srv, "GET", "/api/overview", []*http.Cookie{sess}, ""); rec.Code != http.StatusOK {
		t.Fatalf("OAuth session is not usable: got %d", rec.Code)
	}
}
