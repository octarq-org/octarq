package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
)

// TestOAuthCallbackURLComesFromTheRequest covers the behaviour change operators
// have to know about: redirect_uri is no longer one configured value, it is
// derived from the hostname the login started on — and only from one this
// instance has registered.
//
// A forged Host must not reach the provider at all. redirect_uri is the address
// the provider sends the user back to WITH the authorization code, so a
// redirect_uri an attacker chose is the OAuth-shaped form of the same takeover
// the password-reset link guards against. (Providers reject unregistered
// redirect_uris themselves, but that is their check, not ours.)
func TestOAuthCallbackURLComesFromTheRequest(t *testing.T) {
	db := testDB(t)
	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(testEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	m := New(cfg, cipher).WithDB(db)
	InitGothStore("secret")

	enc, err := cipher.Encrypt([]byte("google-secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	db.Where("key LIKE ?", "oauth.%").Delete(&models.Setting{})
	db.Create(&models.Setting{Key: "oauth.google.client_id", Value: "google-id"})
	db.Create(&models.Setting{Key: "oauth.google.client_secret", Value: enc})

	db.Where("1 = 1").Delete(&dns.Domain{})
	if err := db.Create(&dns.Domain{OrgID: 1, Name: "acme.example"}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	h := NewOAuthHandler(db, m, cipher)

	begin := func(host string, secure bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/auth/begin/google", nil)
		req.SetPathValue("provider", "google")
		req.Host = host
		if secure {
			req.TLS = &tls.ConnectionState{}
		}
		rec := httptest.NewRecorder()
		h.Begin(rec, req)
		return rec
	}

	t.Run("registered host becomes the callback origin", func(t *testing.T) {
		rec := begin("app.acme.example", false)
		if rec.Code != http.StatusTemporaryRedirect && rec.Code != http.StatusFound {
			t.Fatalf("begin: got %d (%s), want a redirect to the provider", rec.Code, rec.Body.String())
		}
		loc, err := url.Parse(rec.Header().Get("Location"))
		if err != nil {
			t.Fatalf("parse Location: %v", err)
		}
		if got, want := loc.Query().Get("redirect_uri"), "https://app.acme.example/auth/callback/google"; got != want {
			t.Errorf("redirect_uri = %q, want %q", got, want)
		}
	})

	// The state cookie carries the CSRF nonce for the round-trip. Marked Secure
	// over plain HTTP the browser drops it and the callback fails with "could
	// not find a matching session"; unmarked over HTTPS the nonce is cleartext.
	t.Run("state cookie Secure follows the request scheme", func(t *testing.T) {
		for _, secure := range []bool{false, true} {
			rec := begin("app.acme.example", secure)
			cookies := rec.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatalf("secure=%v: begin set no state cookie", secure)
			}
			if got := cookies[0].Secure; got != secure {
				t.Errorf("secure=%v: state cookie Secure = %v", secure, got)
			}
		}
	})

	t.Run("forged host cannot start a login", func(t *testing.T) {
		rec := begin("evil.com", false)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("begin on an unregistered host: got %d (%s), want 503", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Header().Get("Location"), "evil.com") {
			t.Error("a forged host reached the provider as a callback address")
		}
	})
}
