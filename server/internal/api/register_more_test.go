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

	// 1. Registration disabled -> 403
	db.Save(&models.Setting{Key: "allow_registration", Value: "false"})
	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"new@example.com","password":"password123"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled registration: got %d, want 403", rec.Code)
	}

	// 2. Enable registration
	db.Save(&models.Setting{Key: "allow_registration", Value: "true"})

	// 3. Email verification required but mail not ready -> 503
	db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"})
	h.registerLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"new@example.com","password":"password123"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("verification required but mail not ready: got %d, want 503", rec.Code)
	}

	// 4. With a mail sender the sign-up is accepted. It answers "a message is
	// on its way" and issues no session: the account is activated from the
	// mailbox, and the answer must not say whether the address was free.
	enableMail(t, db)
	h.registerLimiter.reset("192.0.2.1")

	// Invalid email -> 400
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"not-an-email","password":"password123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid email: got %d, want 400", rec.Code)
	}

	// Long org name > 255 -> 400
	h.registerLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"valid@example.com","password":"password123","orgName":"`+strings.Repeat("a", 260)+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("long org name: got %d, want 400", rec.Code)
	}

	// Valid registration
	h.registerLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"valid@example.com","password":"password123","orgName":"My Custom Team"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid registration: got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("register must not issue a session: the next step is in the mailbox")
	}

	// A duplicate address is not an error: the same answer comes back, and no
	// second account is made.
	h.registerLimiter.reset("192.0.2.1")
	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"valid@example.com","password":"password123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate email register: got %d, want 200", rec.Code)
	}

	// 5. Nil Ctx call
	if _, err := h.register(context.Background(), &RegisterInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in register")
	}
}
