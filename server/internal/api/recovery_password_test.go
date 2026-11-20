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

var errUserWriteInjected = errors.New("injected users-table write failure")

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

func TestResetPasswordKeepsSessionsWhenPasswordWriteFails(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"fail@reset.com","password":"oldpassword123"}`)
	if regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", regRec.Code, regRec.Body.String())
	}
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

	var sessionsAfter int64
	db.Model(&models.Session{}).Where("user_id = ?", user.ID).Count(&sessionsAfter)
	if sessionsAfter != sessionsBefore {
		t.Errorf("sessions revoked despite the password write failing: had %d, now %d — resetPassword must confirm the write before revoking", sessionsBefore, sessionsAfter)
	}

	restore()
	if meRec := do(srv, "GET", "/api/auth/me", cookies, ""); meRec.Code != http.StatusOK {
		t.Errorf("old session after failed reset: got %d, want 200 — the user was logged out by a reset that did not happen", meRec.Code)
	}

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

func TestForgotPasswordFailsLoudlyWhenTokenWriteFails(t *testing.T) {
	srv, db := newTestHandler(t)
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

	var user models.User
	if err := db.Where("email = ?", "fail@forgot.com").First(&user).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	if user.ResetTokenHash != "" {
		t.Error("reset token hash was persisted despite the write being rejected")
	}

	body := strings.ToLower(rec.Body.String())
	for _, leak := range []string{"fail@forgot.com", "not found", "no such", "unknown user", "does not exist"} {
		if strings.Contains(body, leak) {
			t.Errorf("failure body leaks account existence (%q): %s", leak, rec.Body.String())
		}
	}
}

func TestForgotPasswordNoLeak(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	do(srv, "POST", "/api/auth/register", nil, `{"email":"exist@user.com","password":"password123"}`)

	rec1 := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"unknown@user.com"}`)
	if rec1.Code != http.StatusOK {
		t.Fatalf("forgot non-existent: got %d, want 200", rec1.Code)
	}

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

func TestResetPasswordFlow(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"user@reset.com","password":"oldpassword123"}`)
	if regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d", regRec.Code)
	}
	regRec = do(srv, "POST", "/api/auth/login", nil, `{"email":"user@reset.com","password":"oldpassword123"}`)
	if regRec.Code != http.StatusOK {
		t.Fatalf("login: got %d", regRec.Code)
	}
	cookies := regRec.Result().Cookies()

	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("me before reset: got %d, want 200", rec.Code)
	}

	do(srv, "POST", "/api/auth/forgot", nil, `{"email":"user@reset.com"}`)

	var user models.User
	db.Where("email = ?", "user@reset.com").First(&user)

	if rec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"invalid","password":"short"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: got %d, want 400", rec.Code)
	}

	if rec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"wrongtoken","password":"newpassword123"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid token: got %d, want 400", rec.Code)
	}

	expired := time.Now().Add(-1 * time.Hour)
	db.Model(&user).Update("reset_token_expiry", expired)
	if rec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"dummytoken","password":"newpassword123"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expired token: got %d, want 400", rec.Code)
	}

	rawToken, tokenHash, _ := generateSecureToken()
	expiry := time.Now().Add(1 * time.Hour)
	db.Model(&user).Updates(map[string]any{
		"reset_token_hash":   tokenHash,
		"reset_token_expiry": expiry,
	})

	resetRec := do(srv, "POST", "/api/auth/reset", nil, `{"token":"`+rawToken+`","password":"newpassword123"}`)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("valid reset: got %d (%s)", resetRec.Code, resetRec.Body.String())
	}

	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after reset with old session: got %d, want 401", rec.Code)
	}

	loginRec := do(srv, "POST", "/api/auth/login", nil, `{"email":"user@reset.com","password":"newpassword123"}`)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login with new password: got %d (%s)", loginRec.Code, loginRec.Body.String())
	}
}

func TestForgotPasswordRateLimit(t *testing.T) {
	srv, _ := newTestHandler(t)

	for i := 0; i < 5; i++ {
		rec := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"anyone@domain.com"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: got %d, want 200", i+1, rec.Code)
		}
	}

	rec6 := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"anyone@domain.com"}`)
	if rec6.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 6: got %d, want 429", rec6.Code)
	}
}

func TestForgotPasswordNoLeakExistence(t *testing.T) {
	srv, db := newTestHandler(t)
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

func TestForgotPasswordDoesNotExhaustTheLoginBudget(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Create(&models.User{Email: t.Name() + "@x.com", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	for i := 0; i < 12; i++ {
		do(srv, "POST", "/api/auth/forgot", nil, `{"email":"`+t.Name()+`@x.com"}`)
	}

	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"nobody@x.com","password":"wrong"}`)
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("login got 429 after forgot-password spam — the recovery endpoints are still spending the login budget")
	}
}
