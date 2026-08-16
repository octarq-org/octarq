package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// Guard test 1 (core): a TOTP-enabled account arriving through the OAuth
// callback must NOT receive a session. The callback redirects to the same 2FA
// page password login uses, carrying a short-lived challenge; only
// /api/auth/2fa/verify with the code mints the session.
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
	loc, err := url.Parse(cb.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse callback Location: %v", err)
	}
	challenge := loc.Query().Get("twofa")
	if challenge == "" {
		t.Fatalf("callback redirected to %q, want /admin/?twofa=<challenge>", cb.Header().Get("Location"))
	}
	if sess := sessionCookie(cb.Result().Cookies()); sess != nil {
		t.Fatal("OAuth callback set a session cookie while 2FA is pending")
	}

	rec = do(srv, "POST", "/api/auth/2fa/verify", nil, `{"challengeToken":"`+challenge+`","code":"000000"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("challenge without code: got %d, want 401", rec.Code)
	}
	if sess := sessionCookie(rec.Result().Cookies()); sess != nil {
		t.Fatal("challenge without code set a session cookie")
	}

	// The shared verify2FA endpoint completes the login: challenge + TOTP code
	// mints a usable session.
	code, _ = totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil,
		`{"challengeToken":"`+challenge+`","code":"`+code+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa verify via challenge: got %d (%s)", rec.Code, rec.Body.String())
	}
	sess := sessionCookie(rec.Result().Cookies())
	if sess == nil {
		t.Fatal("2fa verify via challenge set no session cookie")
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
	}
	if rec := do(srv, "GET", "/api/overview", []*http.Cookie{sess}, ""); rec.Code != http.StatusOK {
		t.Fatalf("OAuth session is not usable: got %d", rec.Code)
	}
}
