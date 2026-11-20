package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestVerifyEmailRedirectsWithErrorWhenWriteFails(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

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

func TestResendVerificationFailsLoudlyWhenTokenWriteFails(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	if regRec := do(srv, "POST", "/api/auth/register", nil, `{"email":"fail@resend.com","password":"password123"}`); regRec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", regRec.Code, regRec.Body.String())
	}

	failUserWrites(t, db)

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

func TestVerifyEmailAndResendFlow(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	do(srv, "POST", "/api/auth/register", nil, `{"email":"unverified@domain.com","password":"password123"}`)

	var user models.User
	db.Where("email = ?", "unverified@domain.com").First(&user)
	if user.EmailVerified {
		t.Fatal("new registered user should not be email verified by default")
	}

	recInvalid := do(srv, "GET", "/api/auth/verify-email?token=invalidtoken", nil, "")
	if recInvalid.Code != http.StatusFound {
		t.Fatalf("invalid verify token: got %d, want 302", recInvalid.Code)
	}
	if loc := recInvalid.Header().Get("Location"); !strings.Contains(loc, "error=invalid_token") {
		t.Fatalf("expected redirect location with error, got %s", loc)
	}

	recResend := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unverified@domain.com"}`)
	if recResend.Code != http.StatusOK {
		t.Fatalf("resend verification: got %d", recResend.Code)
	}

	db.Where("email = ?", "unverified@domain.com").First(&user)
	if user.VerifyTokenHash == "" {
		t.Fatal("expected VerifyTokenHash after resend")
	}

	rawToken, tokenHash, _ := generateSecureToken()
	expiry := time.Now().Add(24 * time.Hour)
	db.Model(&user).Updates(map[string]any{
		"verify_token_hash":   tokenHash,
		"verify_token_expiry": expiry,
	})

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

func TestEmailVerificationGatingToggle(t *testing.T) {
	srv, db := newTestHandler(t)

	disableEmailVerification(t, db)
	enableMail(t, db)

	do(srv, "POST", "/api/auth/register", nil, `{"email":"gated@user.com","password":"password123"}`)

	recOff := do(srv, "POST", "/api/auth/login", nil, `{"email":"gated@user.com","password":"password123"}`)
	if recOff.Code != http.StatusOK {
		t.Fatalf("login when gating OFF: got %d, want 200", recOff.Code)
	}

	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("save setting: %v", err)
	}

	recOn := do(srv, "POST", "/api/auth/login", nil, `{"email":"gated@user.com","password":"password123"}`)
	if recOn.Code != http.StatusForbidden {
		t.Fatalf("login when gating ON: got %d, want 403 (%s)", recOn.Code, recOn.Body.String())
	}

	db.Model(&models.User{}).Where("email = ?", "gated@user.com").Update("email_verified", true)

	recVerified := do(srv, "POST", "/api/auth/login", nil, `{"email":"gated@user.com","password":"password123"}`)
	if recVerified.Code != http.StatusOK {
		t.Fatalf("login when verified and gating ON: got %d, want 200", recVerified.Code)
	}
}
