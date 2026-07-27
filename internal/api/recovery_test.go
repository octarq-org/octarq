package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
)

// TestForgotPasswordNoLeak verifies that /api/auth/forgot always returns 200
// regardless of whether the email exists, and sets reset tokens for valid users.
func TestForgotPasswordNoLeak(t *testing.T) {
	srv, db := newTestHandler(t)

	// Register a user
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

	// 1. Register user and log in to obtain session
	regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"user@reset.com","password":"oldpassword123"}`)
	if regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d", regRec.Code)
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

	// Re-trigger forgot password to get valid token
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
	loginRec := do(srv, "POST", "/api/auth/login", nil, `{"username":"user@reset.com","password":"newpassword123"}`)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login with new password: got %d (%s)", loginRec.Code, loginRec.Body.String())
	}
}

// TestVerifyEmailAndResendFlow verifies email verification token verification, 302 redirect,
// and resending verification email.
func TestVerifyEmailAndResendFlow(t *testing.T) {
	srv, db := newTestHandler(t)

	// 1. Register user
	do(srv, "POST", "/api/auth/register", nil, `{"email":"unverified@domain.com","password":"password123"}`)

	var user models.User
	db.Where("email = ?", "unverified@domain.com").First(&user)
	if user.EmailVerified {
		t.Fatal("new registered user should not be email verified by default")
	}

	// 2. Invalid verify token -> 302 redirect to /admin/login?error=invalid_token
	recInvalid := do(srv, "GET", "/api/auth/verify-email?token=invalidtoken", nil, "")
	if recInvalid.Code != http.StatusFound {
		t.Fatalf("invalid verify token: got %d, want 302", recInvalid.Code)
	}
	if loc := recInvalid.Header().Get("Location"); !strings.Contains(loc, "error=invalid_token") {
		t.Fatalf("expected redirect location with error, got %s", loc)
	}

	// 3. Resend verification email
	recResend := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unverified@domain.com"}`)
	if recResend.Code != http.StatusOK {
		t.Fatalf("resend verification: got %d", recResend.Code)
	}

	db.Where("email = ?", "unverified@domain.com").First(&user)
	if user.VerifyTokenHash == "" {
		t.Fatal("expected VerifyTokenHash after resend")
	}

	// Manually set a known raw token
	rawToken, tokenHash, _ := generateSecureToken()
	expiry := time.Now().Add(24 * time.Hour)
	db.Model(&user).Updates(map[string]any{
		"verify_token_hash":   tokenHash,
		"verify_token_expiry": expiry,
	})

	// 4. Valid verify token -> 302 redirect to /admin/login?verified=1 and EmailVerified=true
	recValid := do(srv, "GET", "/api/auth/verify-email?token="+rawToken, nil, "")
	if recValid.Code != http.StatusFound {
		t.Fatalf("valid verify token: got %d, want 302", recValid.Code)
	}
	if loc := recValid.Header().Get("Location"); !strings.Contains(loc, "verified=1") {
		t.Fatalf("expected redirect location verified=1, got %s", loc)
	}

	db.Where("email = ?", "unverified@domain.com").First(&user)
	if !user.EmailVerified {
		t.Fatal("expected EmailVerified to be true after successful verification")
	}
}

// TestEmailVerificationGatingToggle verifies require_email_verification setting ON vs OFF.
func TestEmailVerificationGatingToggle(t *testing.T) {
	srv, db := newTestHandler(t)

	// Register user
	do(srv, "POST", "/api/auth/register", nil, `{"email":"gated@user.com","password":"password123"}`)

	// 1. By default (setting OFF): unverified user can log in
	recOff := do(srv, "POST", "/api/auth/login", nil, `{"username":"gated@user.com","password":"password123"}`)
	if recOff.Code != http.StatusOK {
		t.Fatalf("login when gating OFF: got %d, want 200", recOff.Code)
	}

	// 2. Turn gating ON
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("save setting: %v", err)
	}

	// 3. Unverified user login rejected -> 403
	recOn := do(srv, "POST", "/api/auth/login", nil, `{"username":"gated@user.com","password":"password123"}`)
	if recOn.Code != http.StatusForbidden {
		t.Fatalf("login when gating ON: got %d, want 403 (%s)", recOn.Code, recOn.Body.String())
	}

	// 4. Mark user verified
	db.Model(&models.User{}).Where("email = ?", "gated@user.com").Update("email_verified", true)

	// 5. Verified user login succeeds -> 200
	recVerified := do(srv, "POST", "/api/auth/login", nil, `{"username":"gated@user.com","password":"password123"}`)
	if recVerified.Code != http.StatusOK {
		t.Fatalf("login when verified and gating ON: got %d, want 200", recVerified.Code)
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
	srv, _ := newTestHandler(t)

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

// TestPrimaryOrgForUserDeterministic verifies that primaryOrgForUser returns
// the lowest orgID deterministically for multi-org users.
func TestPrimaryOrgForUserDeterministic(t *testing.T) {
	_, db := newTestHandler(t)
	h := &Handler{db: db}

	user := models.User{Email: "multiorg@domain.com", PasswordHash: "xxx"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Add user to org 10 first, then org 5
	db.Create(&models.OrgMember{OrgID: 10, UserID: user.ID, Role: "member"})
	db.Create(&models.OrgMember{OrgID: 5, UserID: user.ID, Role: "admin"})

	for i := 0; i < 5; i++ {
		got := h.primaryOrgForUser(user.ID)
		if got != 5 {
			t.Fatalf("iteration %d: got orgID %d, want 5 (lowest orgID)", i+1, got)
		}
	}
}
