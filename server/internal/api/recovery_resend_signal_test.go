package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	mailmodels "github.com/octarq-org/octarq/server/plugins/mail"
	"gorm.io/gorm"
)

// seedUser inserts a user row directly, for tests that need an account on an
// instance whose sign-up path is closed (no mail sender) or irrelevant.
func seedUser(t *testing.T, db *gorm.DB, email string, verified bool) {
	t.Helper()
	u := models.User{Email: email, EmailVerified: verified, PasswordHash: "x"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
}

// TestResendVerificationMailSignalAndAntiEnumeration verifies that /api/auth/resend-verification
// reports mailConfigured=false when no SMTP sender is configured (even if the mail plugin is mounted)
// or when the mail plugin is not mounted, and reports mailConfigured=true only when an SMTP sender is
// present. Crucially, across unknown, unverified, verified, and empty email inputs, the status code
// and response body MUST be identical byte-for-byte to prevent email enumeration.
func TestResendVerificationMailSignalAndAntiEnumeration(t *testing.T) {
	t.Run("MailPluginMountedWithoutSMTPSender", func(t *testing.T) {
		h, srv, db := newTestHandlerRaw(t)
		disableEmailVerification(t, db)

		// Real-world scenario: mail plugin is mounted in standard build, but no SMTP sender is configured.
		if h.mailReady() {
			t.Fatal("expected mailReady to be false when no SMTP sender is configured")
		}

		// Seed the accounts directly: an instance that cannot send mail does
		// not accept sign-ups any more, and this test is about the resend
		// endpoint, not about how an account comes into being.
		seedUser(t, db, "unverified@nosmtp.com", false)
		seedUser(t, db, "verified@nosmtp.com", true)

		recUnknown := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unknown@nosmtp.com"}`)
		recUnverified := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unverified@nosmtp.com"}`)
		recVerified := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"verified@nosmtp.com"}`)
		recEmpty := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":""}`)

		for name, rec := range map[string]int{
			"unknown":    recUnknown.Code,
			"unverified": recUnverified.Code,
			"verified":   recVerified.Code,
			"empty":      recEmpty.Code,
		} {
			if rec != http.StatusOK {
				t.Errorf("[%s] got status %d, want 200", name, rec)
			}
		}

		// Strict anti-enumeration check: response bodies must be byte-for-byte identical.
		if recUnknown.Body.String() != recUnverified.Body.String() {
			t.Fatalf("body mismatch between unknown and unverified:\nunknown: %s\nunverified: %s",
				recUnknown.Body.String(), recUnverified.Body.String())
		}
		if recUnverified.Body.String() != recVerified.Body.String() {
			t.Fatalf("body mismatch between unverified and verified:\nunverified: %s\nverified: %s",
				recUnverified.Body.String(), recVerified.Body.String())
		}
		if recEmpty.Body.String() != recUnverified.Body.String() {
			t.Fatalf("body mismatch between empty and unverified:\nempty: %s\nunverified: %s",
				recEmpty.Body.String(), recUnverified.Body.String())
		}

		var body ResendVerificationOutputBody
		if err := json.Unmarshal(recUnverified.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if !body.OK {
			t.Errorf("expected ok=true, got false")
		}
		if body.MailConfigured {
			t.Errorf("expected mailConfigured=false when no SMTP sender is configured, got true")
		}
	})

	t.Run("MailPluginMountedWithSMTPSender", func(t *testing.T) {
		h, srv, db := newTestHandlerRaw(t)
		disableEmailVerification(t, db)
		enableMail(t, db)

		// Seed an SMTP sender
		if err := db.Create(&mailmodels.SMTPSender{
			OrgID: 1, Name: "relay", Host: "127.0.0.1", Port: 25,
			User: "u", Pass: "enc", FromEmail: "noreply@example.com",
		}).Error; err != nil {
			t.Fatalf("seed smtp sender: %v", err)
		}

		if !h.mailReady() {
			t.Fatal("expected mailReady to be true when SMTP sender is configured")
		}

		if reg := do(srv, "POST", "/api/auth/register", nil, `{"email":"unverified@withsmtp.com","password":"password123"}`); reg.Code != http.StatusOK {
			t.Fatalf("register unverified user: %d (%s)", reg.Code, reg.Body.String())
		}
		if reg := do(srv, "POST", "/api/auth/register", nil, `{"email":"verified@withsmtp.com","password":"password123"}`); reg.Code != http.StatusOK {
			t.Fatalf("register verified user: %d (%s)", reg.Code, reg.Body.String())
		}
		if err := db.Model(&models.User{}).Where("email = ?", "verified@withsmtp.com").Update("email_verified", true).Error; err != nil {
			t.Fatalf("mark user verified: %v", err)
		}

		recUnknown := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unknown@withsmtp.com"}`)
		recUnverified := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unverified@withsmtp.com"}`)
		recVerified := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"verified@withsmtp.com"}`)
		recEmpty := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":""}`)

		for name, rec := range map[string]int{
			"unknown":    recUnknown.Code,
			"unverified": recUnverified.Code,
			"verified":   recVerified.Code,
			"empty":      recEmpty.Code,
		} {
			if rec != http.StatusOK {
				t.Errorf("[%s] got status %d, want 200", name, rec)
			}
		}

		// Strict anti-enumeration check: response bodies must be byte-for-byte identical.
		if recUnknown.Body.String() != recUnverified.Body.String() {
			t.Fatalf("body mismatch between unknown and unverified:\nunknown: %s\nunverified: %s",
				recUnknown.Body.String(), recUnverified.Body.String())
		}
		if recUnverified.Body.String() != recVerified.Body.String() {
			t.Fatalf("body mismatch between unverified and verified:\nunverified: %s\nverified: %s",
				recUnverified.Body.String(), recVerified.Body.String())
		}
		if recEmpty.Body.String() != recUnverified.Body.String() {
			t.Fatalf("body mismatch between empty and unverified:\nempty: %s\nunverified: %s",
				recEmpty.Body.String(), recUnverified.Body.String())
		}

		var body ResendVerificationOutputBody
		if err := json.Unmarshal(recUnverified.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if !body.OK {
			t.Errorf("expected ok=true, got false")
		}
		if !body.MailConfigured {
			t.Errorf("expected mailConfigured=true when SMTP sender is configured, got false")
		}
	})

	t.Run("WithoutMailPlugin", func(t *testing.T) {
		h, srv, db := newTestHandlerWithoutMail(t)
		disableEmailVerification(t, db)

		if h.mailReady() {
			t.Fatal("expected mailReady to be false when mail plugin is not mounted")
		}

		// Seed the accounts directly: an instance that cannot send mail does
		// not accept sign-ups any more, and this test is about the resend
		// endpoint, not about how an account comes into being.
		seedUser(t, db, "unverified@nomail.com", false)
		seedUser(t, db, "verified@nomail.com", true)

		recUnknown := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unknown@nomail.com"}`)
		recUnverified := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"unverified@nomail.com"}`)
		recVerified := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":"verified@nomail.com"}`)
		recEmpty := do(srv, "POST", "/api/auth/resend-verification", nil, `{"email":""}`)

		for name, rec := range map[string]int{
			"unknown":    recUnknown.Code,
			"unverified": recUnverified.Code,
			"verified":   recVerified.Code,
			"empty":      recEmpty.Code,
		} {
			if rec != http.StatusOK {
				t.Errorf("[%s] got status %d, want 200", name, rec)
			}
		}

		// Strict anti-enumeration check: response bodies must be byte-for-byte identical.
		if recUnknown.Body.String() != recUnverified.Body.String() {
			t.Fatalf("body mismatch between unknown and unverified:\nunknown: %s\nunverified: %s",
				recUnknown.Body.String(), recUnverified.Body.String())
		}
		if recUnverified.Body.String() != recVerified.Body.String() {
			t.Fatalf("body mismatch between unverified and verified:\nunverified: %s\nverified: %s",
				recUnverified.Body.String(), recVerified.Body.String())
		}
		if recEmpty.Body.String() != recUnverified.Body.String() {
			t.Fatalf("body mismatch between empty and unverified:\nempty: %s\nunverified: %s",
				recEmpty.Body.String(), recUnverified.Body.String())
		}

		var body ResendVerificationOutputBody
		if err := json.Unmarshal(recUnverified.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if !body.OK {
			t.Errorf("expected ok=true, got false")
		}
		if body.MailConfigured {
			t.Errorf("expected mailConfigured=false when mail plugin is not mounted, got true")
		}
	})
}
