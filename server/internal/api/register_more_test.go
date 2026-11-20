package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestRegisterMore(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	db.Save(&models.Setting{Key: "allow_registration", Value: "false"})
	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"new@example.com","password":"password123"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled registration: got %d, want 403", rec.Code)
	}

	db.Save(&models.Setting{Key: "allow_registration", Value: "true"})

	db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"})
	h.registerLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"new@example.com","password":"password123"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("verification required but mail not ready: got %d, want 503", rec.Code)
	}

	enableMail(t, db)
	h.registerLimiter.reset("192.0.2.1")

	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"not-an-email","password":"password123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid email: got %d, want 400", rec.Code)
	}

	h.registerLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"valid@example.com","password":"password123","orgName":"`+strings.Repeat("a", 260)+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("long org name: got %d, want 400", rec.Code)
	}

	h.registerLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"valid@example.com","password":"password123","orgName":"My Custom Team"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid registration: got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("register must not issue a session: the next step is in the mailbox")
	}

	h.registerLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"valid@example.com","password":"password123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate email register: got %d, want 200", rec.Code)
	}

	if _, err := h.register(context.Background(), &RegisterInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in register")
	}
}
