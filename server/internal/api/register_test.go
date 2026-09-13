package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	mailmodels "github.com/octarq-org/octarq/plugins/mail"
)

// TestRegisterCreatesUserOrgAndSession verifies the public sign-up path:
// a fresh email/password creates a user, provisions an owner workspace, and
// returns a working session cookie.
func TestRegisterCreatesUserOrgAndSession(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)

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
	srv, db := newTestHandler(t)
	// A mail-less test instance cannot pass the verification gate (register
	// 503s), which is not what this test is about — opt out explicitly.
	disableEmailVerification(t, db)

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

// TestRegisterUsesOrgNameOverEmail pins the optional orgName field: a non-blank
// orgName becomes the provisioned workspace's name; omitting it falls back to
// the registration email, exactly as before the field existed. The gate is
// disabled because a mail-less test instance cannot pass it (register 503s) —
// the org provisioning is what this test pins, not the gate.
func TestRegisterUsesOrgNameOverEmail(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"named@user.com","password":"hunter2pw","orgName":"  Acme Corp  "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register with orgName: got %d (%s)", rec.Code, rec.Body.String())
	}
	var named models.Org
	if err := db.Where("name = ?", "Acme Corp").First(&named).Error; err != nil {
		t.Fatalf("org named after orgName not created: %v", err)
	}

	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"fallback@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register without orgName: got %d (%s)", rec.Code, rec.Body.String())
	}
	var fallback models.Org
	if err := db.Where("name = ?", "fallback@user.com").First(&fallback).Error; err != nil {
		t.Fatalf("org falling back to email not created: %v", err)
	}

	// An oversized orgName is rejected before any row is written.
	long := strings.Repeat("x", 256)
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"long@user.com","password":"hunter2pw","orgName":"`+long+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("register with oversized orgName: got %d, want 400", rec.Code)
	}
}

// TestDBUserPasswordLogin verifies that a registered (non-admin) user can log
// in with their email + password, not just via the admin credential or OAuth.
func TestDBUserPasswordLogin(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)

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
// instance requires a verified email AND can send mail (an SMTP sender is
// configured), sign-up must NOT hand out a session — otherwise the new user
// gets in once and is locked out the moment they log out (login rejects the
// same unverified user, auth.go:74). The sender matters: on an instance that
// cannot send mail, register fails with 503 instead (see
// TestRegisterFailsLoudlyWhenMailUnavailable), because a verificationRequired
// answer would promise an email that can never arrive.
func TestRegisterWithVerificationGateGivesNoSession(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	// A configured sender makes the mail.ready service answer true, so the
	// gate is exercised through its verificationRequired branch rather than
	// the no-mail 503 branch.
	if err := db.Create(&mailmodels.SMTPSender{OrgID: 1, Name: "test", Host: "smtp.example.com", Port: 587, User: "u", Pass: "enc", FromEmail: "noreply@example.com"}).Error; err != nil {
		t.Fatalf("seed smtp sender: %v", err)
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

// TestRegisterFailsLoudlyWhenMailUnavailable pins the fix for the sign-up
// dead end: on a fresh instance (verification required, no SMTP sender
// configured), registration must fail with a clear 503 instead of returning a
// verificationRequired that can never be fulfilled — the verification email
// cannot be sent, so the account would be locked out forever.
func TestRegisterFailsLoudlyWhenMailUnavailable(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	// Deliberately no SMTPSender row: the mail plugin IS mounted (newTestHandler
	// mounts it), but an instance with no sender cannot deliver a single
	// message — exactly the fresh `docker run` shape.

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"stuck@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("register on mail-less instance: got %d (%s), want 503", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cannot send email") {
		t.Fatalf("register 503 does not explain the failure to the user: %s", rec.Body.String())
	}

	// No half-created account may linger behind the error.
	var count int64
	db.Model(&models.User{}).Where("email = ?", "stuck@user.com").Count(&count)
	if count != 0 {
		t.Fatal("register 503 still created an account")
	}
}

// TestRegisterRateLimitedPerIP pins the fix for the phantom registration
// budget: sign-ups used to share loginLimiter, which only Peek()s and counts
// failures, and a successful registration even reset it — so one IP could
// register indefinitely. Registration now has its own 5/hour budget and counts
// every request.
func TestRegisterRateLimitedPerIP(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db) // exercise the full success path; no mail needed

	for i := 0; i < 5; i++ {
		rec := do(srv, "POST", "/api/auth/register", nil, fmt.Sprintf(`{"email":"user%d@rate.test","password":"hunter2pw"}`, i))
		if rec.Code != http.StatusOK {
			t.Fatalf("register %d: got %d (%s), want 200", i+1, rec.Code, rec.Body.String())
		}
	}

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"user5@rate.test","password":"hunter2pw"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6th register from the same IP: got %d, want 429", rec.Code)
	}
}
