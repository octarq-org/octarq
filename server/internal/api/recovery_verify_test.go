package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
)

// The verification link is a 302 flow. A failed write must redirect to the login
// page with an error, not claim verified=1 and not switch to a JSON error.
func TestVerifyEmailRedirectsWithErrorWhenWriteFails(t *testing.T) {
	srv, db := newTestHandler(t)
	// Register is pure setup here; a mail-less test instance cannot pass the
	// verification gate (register 503s), so opt out explicitly.
	disableEmailVerification(t, db)

	if regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"fail@verify.com","password":"password123"}`); regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", regRec.Code, regRec.Body.String())
	}

	var user models.User
	if err := db.Where("email = ?", "fail@verify.com").First(&user).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	rawToken, tokenHash, _ := generateSecureToken()
	if err := db.Model(&user).Updates(map[string]any{
		"verify_token_hash":   tokenHash,
		"verify_token_expiry": time.Now().Add(24 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed verify token: %v", err)
	}

	restore := failUserWrites(t, db)

	rec := do(srv, "GET", "/api/auth/verify-email?token="+rawToken, nil, "")
	if rec.Code != http.StatusFound {
		t.Fatalf("verify-email with a failing write: got %d, want 302 (the flow must stay a redirect)", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if strings.Contains(loc, "verified=1") {
		t.Errorf("verify-email claimed success after a failed write: %s", loc)
	}
	if !strings.HasPrefix(loc, "/admin/login?error=") {
		t.Errorf("expected a /admin/login?error=... redirect, got %s", loc)
	}

	restore()
	var after models.User
	if err := db.Where("email = ?", "fail@verify.com").First(&after).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if after.EmailVerified {
		t.Error("email marked verified even though the write was rejected")
	}
}

// Same contract as forgotPassword, for the resend path.
func TestResendVerificationFailsLoudlyWhenTokenWriteFails(t *testing.T) {
	srv, db := newTestHandler(t)
	// Register is pure setup here; a mail-less test instance cannot pass the
	// verification gate (register 503s), so opt out explicitly.
	disableEmailVerification(t, db)

	if regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"fail@resend.com","password":"password123"}`); regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", regRec.Code, regRec.Body.String())
	}

	failUserWrites(t, db)

	// Unknown addresses must still return 200 with the fault active — the failure
	// below has to come from the write, not from the lookup.
	if rec := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"nobody@resend.com"}`); rec.Code != http.StatusOK {
		t.Fatalf("resend for unknown address: got %d, want 200", rec.Code)
	}

	rec := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"fail@resend.com"}`)
	if rec.Code == http.StatusOK {
		t.Errorf("resend with a failing token write: got 200 (%s), want a non-200 status", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Errorf("resend claimed ok:true while the token write failed: %s", rec.Body.String())
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "fail@resend.com") {
		t.Errorf("failure body names the account: %s", rec.Body.String())
	}
}

// TestVerifyEmailAndResendFlow verifies email verification token verification, 302 redirect,
// and resending verification email.
func TestVerifyEmailAndResendFlow(t *testing.T) {
	srv, db := newTestHandler(t)
	// Register is pure setup here; a mail-less test instance cannot pass the
	// verification gate (register 503s), so opt out explicitly.
	disableEmailVerification(t, db)

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

	// The setting defaults ON (absent → verification required); turn it off to
	// exercise the unverified-login path, exactly as an operator who opts out
	// would.
	disableEmailVerification(t, db)

	do(srv, "POST", "/api/auth/register", nil, `{"email":"gated@user.com","password":"password123"}`)

	// 1. With the gate explicitly OFF: unverified user can log in
	recOff := do(srv, "POST", "/api/auth/login", nil, `{"email":"gated@user.com","password":"password123"}`)
	if recOff.Code != http.StatusOK {
		t.Fatalf("login when gating OFF: got %d, want 200", recOff.Code)
	}

	// 2. Turn gating ON
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("save setting: %v", err)
	}

	// 3. Unverified user login rejected -> 403
	recOn := do(srv, "POST", "/api/auth/login", nil, `{"email":"gated@user.com","password":"password123"}`)
	if recOn.Code != http.StatusForbidden {
		t.Fatalf("login when gating ON: got %d, want 403 (%s)", recOn.Code, recOn.Body.String())
	}

	// 4. Mark user verified
	db.Model(&models.User{}).Where("email = ?", "gated@user.com").Update("email_verified", true)

	// 5. Verified user login succeeds -> 200
	recVerified := do(srv, "POST", "/api/auth/login", nil, `{"email":"gated@user.com","password":"password123"}`)
	if recVerified.Code != http.StatusOK {
		t.Fatalf("login when verified and gating ON: got %d, want 200", recVerified.Code)
	}
}
