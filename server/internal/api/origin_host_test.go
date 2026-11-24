package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func originTestServer(t *testing.T, hosts ...models.OrgHost) (http.Handler, *gorm.DB, *[]string) {
	t.Helper()
	h, srv, db := newTestHandlerRaw(t)

	var sent []string
	send := plugin.SystemMailSender(func(to, subject, htmlBody, textBody string) error {
		sent = append(sent, textBody)
		return nil
	})
	h.SetServiceLookup(func(name string) (any, bool) {
		if name == plugin.ServiceMailSendSystem {
			return send, true
		}
		return nil, false
	})

	for i := range hosts {
		if err := db.Create(&hosts[i]).Error; err != nil {
			t.Fatalf("seed host: %v", err)
		}
	}
	if err := db.Create(&models.User{Email: "victim@example.com", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return srv, db, &sent
}

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
	start := strings.LastIndexAny(body[:i], " \n\t") + 1
	return strings.TrimSpace(body[start:])
}

func TestForgotPasswordRejectsForgedHost(t *testing.T) {
	acme := []models.OrgHost{
		{OrgID: 1, Host: "acme.example", Source: "test"},
		{OrgID: 1, Host: "go.acme.example", Traffic: true, Source: "test"},
	}

	t.Run("forged host is never used", func(t *testing.T) {
		srv, _, sent := originTestServer(t, acme...)

		link := forgot(t, srv, sent, "evil.com")

		if strings.Contains(link, "evil.com") {
			t.Fatalf("password-reset link points at the forged Host: %s\n"+
				"a valid reset token was just mailed to the victim aimed at the attacker's server (CWE-640)", link)
		}
		if !strings.HasPrefix(link, "/admin/reset?token=") {
			t.Errorf("unrecognised host produced %q, want a relative /admin/reset path", link)
		}
	})

	t.Run("registered host is honoured", func(t *testing.T) {
		srv, _, sent := originTestServer(t, acme...)

		link := forgot(t, srv, sent, "app.acme.example")

		if !strings.HasPrefix(link, "https://app.acme.example/admin/reset?token=") {
			t.Errorf("registered host produced %q, want https://app.acme.example/admin/reset?token=…", link)
		}
	})

	t.Run("registered link host is honoured", func(t *testing.T) {
		srv, _, sent := originTestServer(t, acme...)

		link := forgot(t, srv, sent, "go.acme.example")

		if !strings.HasPrefix(link, "https://go.acme.example/admin/reset?token=") {
			t.Errorf("registered link host produced %q, want https://go.acme.example/…", link)
		}
	})

	t.Run("a near-miss of a registered host is still forged", func(t *testing.T) {
		srv, _, sent := originTestServer(t, acme...)

		link := forgot(t, srv, sent, "acme.example.evil.com")

		if strings.Contains(link, "evil.com") {
			t.Fatalf("a hostname that merely CONTAINS a registered domain was accepted: %s", link)
		}
	})

	t.Run("instance with no registered domain falls back to loopback host only", func(t *testing.T) {
		srv, _, sent := originTestServer(t)

		link := forgot(t, srv, sent, "localhost:8080")
		if !strings.HasPrefix(link, "http://localhost:8080/admin/reset?token=") {
			t.Errorf("unregistered instance with localhost produced %q, want http://localhost:8080/admin/reset?token=...", link)
		}

		*sent = nil
		untrustedLink := forgot(t, srv, sent, "octarq.internal:8080")
		if strings.Contains(untrustedLink, "octarq.internal") {
			t.Fatalf("untrusted host header poisoned password-reset link: %s", untrustedLink)
		}
		if !strings.HasPrefix(untrustedLink, "/admin/reset?token=") {
			t.Errorf("untrusted host produced %q, want relative /admin/reset path", untrustedLink)
		}
	})
}

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
