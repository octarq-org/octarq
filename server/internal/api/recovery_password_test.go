package api

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

// errUserWriteInjected is the fault the guard tests below push into GORM.
var errUserWriteInjected = errors.New("injected users-table write failure")

// failUserWrites makes every UPDATE against the users table fail, and only that.
// A before-"gorm:update" callback that calls AddError short-circuits gorm's own
// update callback (it returns early on db.Error), so the row is genuinely not
// written — no sleeps, no timing, stable under -race.
//
// Sessions live in their own table and are deleted, not updated, so this fault
// leaves the session revocation path fully functional. That is the point: the
// only thing standing between a failed password write and a logged-out user is
// the error check in resetPassword.
// It returns an idempotent restore func; it is also registered as cleanup, so
// callers only call it when a later assertion needs writes working again.
func failUserWrites(t *testing.T, db *gorm.DB) (restore func()) {
	t.Helper()
	const name = "test:fail_user_writes"
	err := db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "users" {
			tx.AddError(errUserWriteInjected)
		}
	})
	if err != nil {
		t.Fatalf("register failing callback: %v", err)
	}
	done := false
	restore = func() {
		if done {
			return
		}
		done = true
		if err := db.Callback().Update().Remove(name); err != nil {
			t.Errorf("remove failing callback: %v", err)
		}
	}
	t.Cleanup(restore)
	return restore
}

// seedResetToken gives the user a valid, known reset token without going through
// the mail path, and returns the raw token to POST back.
func seedResetToken(t *testing.T, db *gorm.DB, user *models.User) string {
	t.Helper()
	rawToken, tokenHash, err := generateSecureToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	expiry := time.Now().Add(1 * time.Hour)
	if err := db.Model(user).Updates(map[string]any{
		"reset_token_hash":   tokenHash,
		"reset_token_expiry": expiry,
	}).Error; err != nil {
		t.Fatalf("seed reset token: %v", err)
	}
	return rawToken
}

// A failed password write must not cost the user their sessions.
//
// resetPassword used to drop the *gorm.DB returned by Updates and then delete
// every session unconditionally. When the write failed the caller got ok:true,
// the old password still worked, and every device had been logged out — the
// worst of all three outcomes, silently. Revocation is now strictly downstream
// of a confirmed write.
func TestResetPasswordKeepsSessionsWhenPasswordWriteFails(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"fail@reset.com","password":"oldpassword123"}`)
	if regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", regRec.Code, regRec.Body.String())
	}
	// Sign-up no longer hands out a session (the account is activated from the
	// mailbox, and the answer must not say whether the address was free), so
	// the tests that need a session sign in for it.
	regRec = do(srv, "POST", "/api/auth/login", nil, `{"email":"fail@reset.com","password":"oldpassword123"}`)
	if regRec.Code != http.StatusOK {
		t.Fatalf("login: got %d (%s)", regRec.Code, regRec.Body.String())
	}
	cookies := regRec.Result().Cookies()

	var user models.User
	if err := db.Where("email = ?", "fail@reset.com").First(&user).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	rawToken := seedResetToken(t, db, &user)

	var sessionsBefore int64
	db.Model(&models.Session{}).Where("user_id = ?", user.ID).Count(&sessionsBefore)
	if sessionsBefore == 0 {
		t.Fatal("precondition failed: expected at least one session before the reset attempt")
	}

	restore := failUserWrites(t, db)

	rec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"`+rawToken+`","password":"newpassword123"}`)
	if rec.Code >= 200 && rec.Code < 300 {
		t.Errorf("reset with a failing password write: got %d, want a non-2xx status (body %s)", rec.Code, rec.Body.String())
	}

	// The load-bearing assertion.
	var sessionsAfter int64
	db.Model(&models.Session{}).Where("user_id = ?", user.ID).Count(&sessionsAfter)
	if sessionsAfter != sessionsBefore {
		t.Errorf("sessions revoked despite the password write failing: had %d, now %d — resetPassword must confirm the write before revoking", sessionsBefore, sessionsAfter)
	}

	// Same claim from the outside: the old cookie must still authenticate, i.e.
	// the session cache entry was not dropped either.
	restore()
	if meRec := do(srv, "GET", "/api/auth/me", cookies, ""); meRec.Code != http.StatusOK {
		t.Errorf("old session after failed reset: got %d, want 200 — the user was logged out by a reset that did not happen", meRec.Code)
	}

	// And the password really did not change.
	var after models.User
	if err := db.Where("email = ?", "fail@reset.com").First(&after).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if after.PasswordHash != user.PasswordHash {
		t.Error("password hash changed even though the write was rejected")
	}
	if loginRec := do(srv, "POST", "/api/auth/login", nil, `{"email":"fail@reset.com","password":"oldpassword123"}`); loginRec.Code != http.StatusOK {
		t.Errorf("login with the old password after a failed reset: got %d, want 200", loginRec.Code)
	}
}

// A reset token that never reached the database must not be reported as sent.
//
// The 500 is enumeration-safe: unknown addresses short-circuit to 200 before any
// write, so this status is only ever reached after a storage fault, and the body
// says nothing about the account.
func TestForgotPasswordFailsLoudlyWhenTokenWriteFails(t *testing.T) {
	srv, db := newTestHandler(t)
	// Register is pure setup here; a mail-less test instance cannot pass the
	// verification gate (register 503s), so opt out explicitly.
	disableEmailVerification(t, db)
	enableMail(t, db)

	if regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"fail@forgot.com","password":"password123"}`); regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", regRec.Code, regRec.Body.String())
	}

	failUserWrites(t, db)

	rec := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"fail@forgot.com"}`)
	if rec.Code == http.StatusOK {
		t.Errorf("forgot with a failing token write: got 200 (%s), want a non-200 status", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Errorf("forgot claimed ok:true while the token write failed: %s", rec.Body.String())
	}

	// The token must not be half-committed.
	var user models.User
	if err := db.Where("email = ?", "fail@forgot.com").First(&user).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	if user.ResetTokenHash != "" {
		t.Error("reset token hash was persisted despite the write being rejected")
	}

	// Anti-enumeration: nothing in the failure response may name the account or
	// otherwise reveal that it exists.
	body := strings.ToLower(rec.Body.String())
	for _, leak := range []string{"fail@forgot.com", "not found", "no such", "unknown user", "does not exist"} {
		if strings.Contains(body, leak) {
			t.Errorf("failure body leaks account existence (%q): %s", leak, rec.Body.String())
		}
	}
}

// TestForgotPasswordNoLeak verifies that /api/auth/forgot always returns 200
// regardless of whether the email exists, and sets reset tokens for valid users.
func TestForgotPasswordNoLeak(t *testing.T) {
	srv, db := newTestHandler(t)
	// Register is pure setup here; a mail-less test instance cannot pass the
	// verification gate (register 503s), so opt out explicitly.
	disableEmailVerification(t, db)
	enableMail(t, db)

	do(srv, "POST", "/api/auth/register", nil, `{"email":"exist@user.com","password":"password123"}`)

	// Request for non-existent email -> 200 OK
	rec1 := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"unknown@user.com"}`)
	if rec1.Code != http.StatusOK {
		t.Fatalf("forgot non-existent: got %d, want 200", rec1.Code)
	}

	// Request for existing email -> 200 OK and token is set
	rec2 := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"exist@user.com"}`)
	if rec2.Code != http.StatusOK {
		t.Fatalf("forgot existing: got %d, want 200", rec2.Code)
	}

	var user models.User
	if err := db.Where("email = ?", "exist@user.com").First(&user).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	if user.ResetTokenHash == "" {
		t.Fatal("expected ResetTokenHash to be populated")
	}
	if user.ResetTokenExpiry == nil || time.Now().After(*user.ResetTokenExpiry) {
		t.Fatal("expected valid ResetTokenExpiry")
	}
}

// TestResetPasswordFlow verifies resetting password with valid/invalid/expired token
// and session revocation upon reset.
func TestResetPasswordFlow(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	// 1. Register user and log in to obtain session
	regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"user@reset.com","password":"oldpassword123"}`)
	if regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d", regRec.Code)
	}
	// Sign-up no longer hands out a session (the account is activated from the
	// mailbox, and the answer must not say whether the address was free), so
	// the tests that need a session sign in for it.
	regRec = do(srv, "POST", "/api/auth/login", nil, `{"email":"user@reset.com","password":"oldpassword123"}`)
	if regRec.Code != http.StatusOK {
		t.Fatalf("login: got %d", regRec.Code)
	}
	cookies := regRec.Result().Cookies()

	// Verify session is active
	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("me before reset: got %d, want 200", rec.Code)
	}

	// 2. Request forgot password to generate token
	do(srv, "POST", "/api/auth/forgot", nil, `{"email":"user@reset.com"}`)

	var user models.User
	db.Where("email = ?", "user@reset.com").First(&user)

	// 3. Short password -> 400
	if rec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"invalid","password":"short"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: got %d, want 400", rec.Code)
	}

	// 4. Invalid token -> 400
	if rec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"wrongtoken","password":"newpassword123"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid token: got %d, want 400", rec.Code)
	}

	// 5. Expired token -> 400
	expired := time.Now().Add(-1 * time.Hour)
	db.Model(&user).Update("reset_token_expiry", expired)
	if rec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"dummytoken","password":"newpassword123"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expired token: got %d, want 400", rec.Code)
	}

	// Seed a known, unexpired token directly (bypassing the mail path).
	rawToken, tokenHash, _ := generateSecureToken()
	expiry := time.Now().Add(1 * time.Hour)
	db.Model(&user).Updates(map[string]any{
		"reset_token_hash":   tokenHash,
		"reset_token_expiry": expiry,
	})

	// 6. Valid reset -> 200 OK
	resetRec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"`+rawToken+`","password":"newpassword123"}`)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("valid reset: got %d (%s)", resetRec.Code, resetRec.Body.String())
	}

	// 7. Verify previous session was revoked
	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after reset with old session: got %d, want 401", rec.Code)
	}

	// 8. Verify login with new password succeeds
	loginRec := do(srv, "POST", "/api/auth/login", nil, `{"email":"user@reset.com","password":"newpassword123"}`)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login with new password: got %d (%s)", loginRec.Code, loginRec.Body.String())
	}
}

// TestForgotPasswordRateLimit verifies that hitting POST /api/auth/forgot
// repeatedly from the same IP eventually triggers 429 Too Many Requests.
func TestForgotPasswordRateLimit(t *testing.T) {
	srv, _ := newTestHandler(t)

	// Threshold is 5 attempts per 15 minutes.
	for i := 0; i < 5; i++ {
		rec := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"anyone@domain.com"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: got %d, want 200", i+1, rec.Code)
		}
	}

	// 6th attempt -> 429 Too Many Requests
	rec6 := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"anyone@domain.com"}`)
	if rec6.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 6: got %d, want 429", rec6.Code)
	}
}

// TestForgotPasswordNoLeakExistence verifies that POST /api/auth/forgot
// returns identical status code and response body for existent vs non-existent emails.
func TestForgotPasswordNoLeakExistence(t *testing.T) {
	srv, db := newTestHandler(t)
	// Register is pure setup here; a mail-less test instance cannot pass the
	// verification gate (register 503s), so opt out explicitly.
	disableEmailVerification(t, db)
	enableMail(t, db)

	do(srv, "POST", "/api/auth/register", nil, `{"email":"realuser@domain.com","password":"password123"}`)

	recNonExistent := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"fakeuser@domain.com"}`)
	recExistent := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"realuser@domain.com"}`)

	if recNonExistent.Code != http.StatusOK || recExistent.Code != http.StatusOK {
		t.Fatalf("status code mismatch: non-existent=%d, existent=%d, both want 200",
			recNonExistent.Code, recExistent.Code)
	}

	if recNonExistent.Body.String() != recExistent.Body.String() {
		t.Fatalf("response body mismatch:\nnon-existent: %s\nexistent: %s",
			recNonExistent.Body.String(), recExistent.Body.String())
	}
}

// Password-reset spam must not lock anyone out of logging in.
//
// The recovery endpoints originally shared loginLimiter, whose budget is 5 per
// 15 minutes per IP. Counting every reset request against it meant five
// unauthenticated /api/auth/forgot calls would block login and registration for
// everyone behind that IP — a corporate NAT, a shared office — for 15 minutes.
// They now have their own budget.
func TestForgotPasswordDoesNotExhaustTheLoginBudget(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Create(&models.User{Email: t.Name() + "@x.com", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Burn well past the recovery budget.
	for i := 0; i < 12; i++ {
		do(srv, "POST", "/api/auth/forgot", nil, `{"email":"`+t.Name()+`@x.com"}`)
	}

	// Login must still be reachable — 401 for bad credentials is fine, 429 is not.
	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"nobody@x.com","password":"wrong"}`)
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("login got 429 after forgot-password spam — the recovery endpoints are still spending the login budget")
	}
}
