package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

// TestRegisterCreatesUserOrgAndSession verifies the public sign-up path:
// a fresh email/password creates a user, provisions an owner workspace, and
// returns a working session cookie.
func TestRegisterCreatesUserOrgAndSession(t *testing.T) {
	srv, db := newTestHandler(t)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"new@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("register: expected a session cookie")
	}

	var user models.User
	if err := db.Where("email = ?", "new@user.com").First(&user).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.PasswordHash == "" {
		t.Fatal("register: password hash not stored")
	}
	var member models.OrgMember
	if err := db.Where("user_id = ?", user.ID).First(&member).Error; err != nil {
		t.Fatalf("membership not created: %v", err)
	}
	if member.Role != "owner" {
		t.Fatalf("role: got %q, want owner", member.Role)
	}

	// The returned session cookie authenticates against a protected endpoint.
	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("me with fresh session: got %d, want 200", rec.Code)
	}
}

// TestRegisterRejectsDuplicateAndShortPassword covers the guard rails.
func TestRegisterRejectsDuplicateAndShortPassword(t *testing.T) {
	srv, _ := newTestHandler(t)

	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"a@b.com","password":"short"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: got %d, want 400", rec.Code)
	}
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"a@b.com","password":"longenough"}`); rec.Code != http.StatusOK {
		t.Fatalf("first register: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Same email again (case-insensitive) → conflict.
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"A@B.com","password":"longenough"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: got %d, want 409", rec.Code)
	}
}

// TestRegisterDisabledByToggle verifies the allow_registration setting gates
// the endpoint (default on; explicit "false" turns it off).
func TestRegisterDisabledByToggle(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyAllowRegistration, Value: "false"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"x@y.com","password":"longenough"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("register while disabled: got %d, want 403", rec.Code)
	}
}

// TestDBUserPasswordLogin verifies that a registered (non-admin) user can log
// in with their email + password, not just via the admin credential or OAuth.
func TestDBUserPasswordLogin(t *testing.T) {
	srv, _ := newTestHandler(t)

	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"member@corp.com","password":"correcthorse"}`); rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Correct password logs in.
	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"member@corp.com","password":"correcthorse"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("db-user login: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "GET", "/api/auth/me", rec.Result().Cookies(), ""); rec.Code != http.StatusOK {
		t.Fatalf("me after db-user login: got %d, want 200", rec.Code)
	}

	// Wrong password is rejected.
	if rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"member@corp.com","password":"wrongpass"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad password: got %d, want 401", rec.Code)
	}
}

// registerBody is the decoded shape of a POST /api/auth/register response.
// Decoded from the wire, never reconstructed from the handler's own logic.
type registerBody struct {
	OK                   bool   `json:"ok"`
	Email                string `json:"email"`
	VerificationRequired bool   `json:"verificationRequired"`
}

// TestRegisterWithVerificationGateGivesNoSession pins the fix: when the
// instance requires a verified email, sign-up must NOT hand out a session —
// otherwise the new user gets in once and is locked out the moment they log
// out (login rejects the same unverified user, auth.go:72).
func TestRegisterWithVerificationGateGivesNoSession(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"gated@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}

	var body registerBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode register response: %v (%s)", err, rec.Body.String())
	}
	if !body.VerificationRequired {
		t.Fatalf("register with gate on: verificationRequired = false, want true (%s)", rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 0 {
		t.Fatalf("register with gate on: got %d cookie(s), want none", len(cookies))
	}
	// Whatever came back (nothing) must not authenticate a protected endpoint.
	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code == http.StatusOK {
		t.Fatal("register with gate on: response authenticates /api/auth/me, want rejected")
	}

	// The account itself was still created — the user just has to verify.
	var user models.User
	if err := db.Where("email = ?", "gated@user.com").First(&user).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.EmailVerified {
		t.Fatal("fresh account should start unverified")
	}
}

// TestRegisterWithoutVerificationGateKeepsSession is the other direction: with
// the (default-off) setting explicitly off, sign-up still logs the user in.
// This is the guard against over-tightening the fix above.
func TestRegisterWithoutVerificationGateKeepsSession(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "false"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"open@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}

	var body registerBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode register response: %v (%s)", err, rec.Body.String())
	}
	if body.VerificationRequired {
		t.Fatalf("register with gate off: verificationRequired = true, want false (%s)", rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("register with gate off: expected a session cookie")
	}
	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("me with fresh session: got %d, want 200", rec.Code)
	}
}
