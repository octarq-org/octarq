package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

// originTestServer builds the real API server, seeds a user, and captures the
// body of every mail the handlers send, so the assertions below read the link
// that a victim would actually receive.
//
// Nothing here re-implements host validation: the test only forges a Host
// header and reads what came out the other end of the production handler.
func originTestServer(t *testing.T, domains ...dns.Domain) (http.Handler, *gorm.DB, *[]string) {
	t.Helper()
	h, srv, db := newTestHandlerRaw(t)

	var sent []string
	send := plugin.MailSender(func(orgID uint, to, subject, htmlBody, textBody string) error {
		sent = append(sent, textBody)
		return nil
	})
	h.SetServiceLookup(func(name string) (any, bool) {
		if name == plugin.ServiceMailSend {
			return send, true
		}
		return nil, false
	})

	for i := range domains {
		if err := db.Create(&domains[i]).Error; err != nil {
			t.Fatalf("seed domain: %v", err)
		}
	}
	if err := db.Create(&models.User{Email: "victim@example.com", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return srv, db, &sent
}

// forgot fires POST /api/auth/forgot with the given Host header and returns the
// single link that was mailed out.
func forgot(t *testing.T, srv http.Handler, sent *[]string, host string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/auth/forgot", strings.NewReader(`{"email":"victim@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Host = host
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/forgot with Host %q: got %d (%s)", host, rec.Code, rec.Body.String())
	}
	if len(*sent) != 1 {
		t.Fatalf("expected exactly 1 mail sent, got %d — the assertion below would be vacuous", len(*sent))
	}
	body := (*sent)[0]
	i := strings.Index(body, "/admin/reset?token=")
	if i < 0 {
		t.Fatalf("mailed body carries no reset link:\n%s", body)
	}
	// Walk back to the start of the URL (the mail puts it on its own line).
	start := strings.LastIndexAny(body[:i], " \n\t") + 1
	return strings.TrimSpace(body[start:])
}

// TestForgotPasswordRejectsForgedHost is the guard for the attack this refactor
// would otherwise have introduced (CWE-640).
//
// Absolute links used to come from OCTARQ_BASE_URL and were therefore immune to
// anything the caller sent. They now come from the request, so an attacker who
// POSTs /api/auth/forgot with "Host: evil.com" would — without the ownership
// check — have the victim mailed a link carrying a VALID reset token pointed at
// the attacker's server. One click by the victim is a full account takeover, on
// an account the attacker only had to know the email address of.
func TestForgotPasswordRejectsForgedHost(t *testing.T) {
	acme := dns.Domain{
		OrgID:     1,
		Name:      "acme.example",
		ForLink:   true,
		LinkHosts: models.HostList{{Host: "go.acme.example", Enabled: true}},
	}

	t.Run("forged host is never used", func(t *testing.T) {
		srv, _, sent := originTestServer(t, acme)

		link := forgot(t, srv, sent, "evil.com")

		if strings.Contains(link, "evil.com") {
			t.Fatalf("password-reset link points at the forged Host: %s\n"+
				"a valid reset token was just mailed to the victim aimed at the attacker's server (CWE-640)", link)
		}
		// The safe degradation: a relative path. Unopenable from a mail client,
		// which is a support ticket; an attacker's absolute URL is a breach.
		if !strings.HasPrefix(link, "/admin/reset?token=") {
			t.Errorf("unrecognised host produced %q, want a relative /admin/reset path", link)
		}
	})

	t.Run("registered host is honoured", func(t *testing.T) {
		srv, _, sent := originTestServer(t, acme)

		// A subdomain of a registered domain: whoever controls the zone
		// controls this name, so links may be built on it.
		link := forgot(t, srv, sent, "app.acme.example")

		if !strings.HasPrefix(link, "https://app.acme.example/admin/reset?token=") {
			t.Errorf("registered host produced %q, want https://app.acme.example/admin/reset?token=…", link)
		}
	})

	t.Run("registered link host is honoured", func(t *testing.T) {
		srv, _, sent := originTestServer(t, acme)

		link := forgot(t, srv, sent, "go.acme.example")

		if !strings.HasPrefix(link, "https://go.acme.example/admin/reset?token=") {
			t.Errorf("registered link host produced %q, want https://go.acme.example/…", link)
		}
	})

	t.Run("a near-miss of a registered host is still forged", func(t *testing.T) {
		srv, _, sent := originTestServer(t, acme)

		// "acme.example.evil.com" ends with a registered name and would pass a
		// suffix or substring test. It is a domain the attacker owns.
		link := forgot(t, srv, sent, "acme.example.evil.com")

		if strings.Contains(link, "evil.com") {
			t.Fatalf("a hostname that merely CONTAINS a registered domain was accepted: %s", link)
		}
	})

	t.Run("instance with no registered domain falls back to the request host", func(t *testing.T) {
		// Documented fallback: with nothing registered there is no whitelist to
		// check against, and refusing every absolute link would mean password
		// reset never works on a fresh self-hosted instance. This is exactly why
		// the fallback must switch off as soon as a domain exists — the case
		// above.
		srv, _, sent := originTestServer(t)

		link := forgot(t, srv, sent, "octarq.internal:8080")

		if !strings.HasPrefix(link, "http://octarq.internal:8080/admin/reset?token=") {
			t.Errorf("unregistered instance produced %q, want the request host verbatim", link)
		}
	})
}

// TestSessionCookieSecureFollowsRequest pins that the session cookie's Secure
// attribute is decided by the request rather than by configuration.
//
// The old OCTARQ_SECURE_COOKIES could only ever be right for one scheme: set
// over plain HTTP the browser drops the cookie and the user is 401 on every
// request after a "successful" login; unset over HTTPS the session token
// travels in cleartext.
func TestSessionCookieSecureFollowsRequest(t *testing.T) {
	cases := []struct {
		name string
		tls  bool
		want bool
	}{
		{name: "https request marks the cookie Secure", tls: true, want: true},
		{name: "http request does not", tls: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, _ := originTestServer(t)

			req := httptest.NewRequest("POST", "/api/auth/login",
				strings.NewReader(`{"email":"admin","password":"pw"}`))
			req.Header.Set("Content-Type", "application/json")
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("login: got %d (%s)", rec.Code, rec.Body.String())
			}

			cookies := rec.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("login set no cookie; the assertion below would be vacuous")
			}
			if got := cookies[0].Secure; got != tc.want {
				t.Errorf("session cookie Secure = %v, want %v", got, tc.want)
			}
		})
	}
}
