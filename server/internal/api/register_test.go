package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// TestRegisterCreatesUserAndOrg verifies the public sign-up path: a fresh
// email/password creates a user and provisions an owner workspace.
//
// It creates no session. Sign-up always ends in the mailbox — the response
// must not say whether the address already had an account, and a session
// cookie would say exactly that — so the account is activated by the
// verification link, and the user signs in after it.
func TestRegisterCreatesUserAndOrg(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"new@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("register: got %d cookie(s), want none", len(cookies))
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

	// The password still signs the user in — the account is usable, it just
	// was not activated by the sign-up call itself.
	rec = do(srv, "POST", "/api/auth/login", nil, `{"email":"new@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login after register: got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestRegisterHidesAnExistingAccount pins the one rule that shapes this
// endpoint: a sign-up for an address that already has an account must look
// exactly like a sign-up for a free address.
//
// The endpoint is public, so an answer that distinguishes the two turns it
// into an oracle for "who has an account on this instance". Everything the
// caller can see therefore has to match: the status, the body, the headers
// and the cookies.
func TestRegisterHidesAnExistingAccount(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	first := do(srv, "POST", "/api/auth/register", nil, `{"email":"taken@user.com","password":"hunter2pw"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first register: got %d (%s)", first.Code, first.Body.String())
	}
	// The same address again (case-insensitive), a different password.
	second := do(srv, "POST", "/api/auth/register", nil, `{"email":"TAKEN@user.com","password":"differentpw"}`)
	if second.Code != first.Code {
		t.Fatalf("second register: got %d, want %d — the two answers must match", second.Code, first.Code)
	}
	if second.Body.String() != first.Body.String() {
		t.Fatalf("second register body = %s, want %s — the two answers must match", second.Body.String(), first.Body.String())
	}
	if cookies := second.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("second register: got %d cookie(s), want none", len(cookies))
	}
	// And no second account was made: the address is an identity, not a label.
	var count int64
	if err := db.Model(&models.User{}).Where("LOWER(email) = ?", "taken@user.com").Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("user rows for the address: got %d, want 1", count)
	}
	// The second sign-up must not have overwritten the first account's password.
	var user models.User
	if err := db.Where("LOWER(email) = ?", "taken@user.com").First(&user).Error; err != nil {
		t.Fatalf("user not found: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("hunter2pw")) != nil {
		t.Fatal("the second sign-up changed the existing account's password")
	}
}

// TestRegisterRejectsShortPassword covers the input guard rails. The duplicate
// case is not here: a duplicate is not an error any more (see
// TestRegisterHidesAnExistingAccount).
func TestRegisterRejectsShortPassword(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"a@b.com","password":"short"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: got %d, want 400", rec.Code)
	}
}

// TestRegisterDisabledByToggle verifies the allow_registration setting gates
// the endpoint (default on; explicit "false" turns it off).
func TestRegisterDisabledByToggle(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyAllowRegistration, Value: "false"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	enableMail(t, db)
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"x@y.com","password":"longenough"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("register while disabled: got %d, want 403", rec.Code)
	}
}

// TestRegisterUsesOrgNameOverEmail pins the optional orgName field: a non-blank
// orgName becomes the provisioned workspace's name; omitting it falls back to
// the registration email, exactly as before the field existed.
func TestRegisterUsesOrgNameOverEmail(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

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
	enableMail(t, db)
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"long@user.com","password":"hunter2pw","orgName":"`+long+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("register with oversized orgName: got %d, want 400", rec.Code)
	}
}

// TestDBUserPasswordLogin verifies that a registered (non-admin) user can log
// in with their email + password, not just via the admin credential or OAuth.
func TestDBUserPasswordLogin(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)

	enableMail(t, db)
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
	OK    bool   `json:"ok"`
	Email string `json:"email"`
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
	// sign-up proceeds past the no-mail 503 branch.
	enableMail(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"gated@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
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

// TestRegisterWithoutVerificationGateStillGivesNoSession is the other
// direction: with the sign-in verification gate off, sign-up still issues no
// session.
//
// It used to issue one, and that is exactly the leak the uniform answer
// closes: a session cookie would tell "the address was free" apart from "the
// address already had an account" all by itself. The gate is a sign-in gate;
// it does not make the sign-up answer say more than it may.
func TestRegisterWithoutVerificationGateStillGivesNoSession(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"open@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("register with gate off: got %d cookie(s), want none", len(cookies))
	}
}

// TestRegisterFailsLoudlyWhenMailUnavailable pins the sign-up dead end: on an
// instance that cannot send mail, registration must fail with a clear 503
// instead of answering "a message is on its way" — the message cannot be
// sent, so the account would be locked out forever.
//
// The sign-in gate does not change this: sign-up always ends in the mailbox,
// so an instance without a mail sender cannot take sign-ups at all.
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

// TestRegisterWithoutMailAndWithoutGateStillFails is the same dead end with
// the sign-in gate off: the gate is not what makes the mailbox necessary, so
// turning it off must not hand the endpoint a way to promise a message it
// cannot deliver.
func TestRegisterWithoutMailAndWithoutGateStillFails(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"nogate@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("register on mail-less instance with gate off: got %d (%s), want 503", rec.Code, rec.Body.String())
	}
}

// TestRegisterRateLimitedPerIP pins the fix for the phantom registration
// budget: sign-ups used to share loginLimiter, which only Peek()s and counts
// failures, and a successful registration even reset it — so one IP could
// register indefinitely. Registration now has its own 5/hour budget and counts
// every request.
func TestRegisterRateLimitedPerIP(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db) // exercise the full success path

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
