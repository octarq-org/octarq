package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestInstanceMailTestRequiresAuth(t *testing.T) {
	_, srv, _ := newTestHandlerRaw(t)

	req := httptest.NewRequest(http.MethodPost, "/api/instance/mail/test", strings.NewReader(`{"to":"test@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /api/instance/mail/test: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstanceMailTestRequiresInstanceAdmin(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)

	const org = uint(101)
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	req := httptest.NewRequest(http.MethodPost, "/api/instance/mail/test", strings.NewReader(`{"to":"test@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, memberUID, org) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member POST /api/instance/mail/test: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstanceMailTestValidationAndSenderScenarios(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	cookies := loginCookies(t, srv)

	t.Run("invalid recipient email", func(t *testing.T) {
		rec := do(srv, "POST", "/api/instance/mail/test", cookies, `{"to":"not-an-email"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid email, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("service unavailable when no system sender is registered", func(t *testing.T) {
		h.SetServiceLookup(func(name string) (any, bool) {
			return nil, false
		})
		rec := do(srv, "POST", "/api/instance/mail/test", cookies, `{"to":"valid@example.com"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 when system sender missing, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("failed delivery returns 400", func(t *testing.T) {
		h.SetServiceLookup(func(name string) (any, bool) {
			if name == plugin.ServiceMailSendSystem {
				return plugin.SystemMailSender(func(to, subject, htmlBody, textBody string) error {
					return errors.New("smtp relay connection refused")
				}), true
			}
			return nil, false
		})
		rec := do(srv, "POST", "/api/instance/mail/test", cookies, `{"to":"valid@example.com"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "connection refused") {
			t.Fatalf("expected 400 on delivery error, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("successful delivery returns 200", func(t *testing.T) {
		sentTo := ""
		sentSubj := ""
		h.SetServiceLookup(func(name string) (any, bool) {
			if name == plugin.ServiceMailSendSystem {
				return plugin.SystemMailSender(func(to, subject, htmlBody, textBody string) error {
					sentTo = to
					sentSubj = subject
					return nil
				}), true
			}
			return nil, false
		})
		rec := do(srv, "POST", "/api/instance/mail/test", cookies, `{"to":"recipient@example.com"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on success, got %d: %s", rec.Code, rec.Body.String())
		}
		if sentTo != "recipient@example.com" {
			t.Fatalf("sentTo mismatch: got %s", sentTo)
		}
		if !strings.Contains(sentSubj, "Test") {
			t.Fatalf("subject mismatch: got %s", sentSubj)
		}
	})
}
