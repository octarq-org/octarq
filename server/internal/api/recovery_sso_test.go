package api

// POST /api/auth/forgot must not mint a local password for an account whose
// only credential lives at an identity provider.
//
// changePassword and changeEmail already refuse an account with an empty
// PasswordHash. The emailed reset did not, which made it the one door around
// the IdP: issue a reset for an SSO-only address, and whoever redeems the link
// has a local password on an account whose sign-in policy — MFA, device trust,
// deprovisioning — the IdP is supposed to own.
//
// The refusal has to be invisible. This endpoint is unauthenticated and already
// returns 200 {ok:true} for addresses that do not exist at all; if the SSO case
// answered differently it would become an oracle for "is this an SSO account"
// and, worse, for "does this account exist".

import (
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// forgotFor fires the public reset request for one address.
func forgotFor(t *testing.T, srv http.Handler, email string) (int, string) {
	t.Helper()
	rec := do(srv, "POST", "/api/auth/forgot", nil, `{"email":"`+email+`"}`)
	return rec.Code, rec.Body.String()
}

// TestForgotPasswordRefusesSSOOnlyAccount: an account with no stored password
// gets no reset token and no mail — while a password account still does, which
// is what proves the endpoint was not simply broken.
func TestForgotPasswordRefusesSSOOnlyAccount(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	var sent []string
	h.SetServiceLookup(func(name string) (any, bool) {
		if name == plugin.ServiceMailSendSystem {
			return plugin.SystemMailSender(func(to, subject, htmlBody, textBody string) error {
				sent = append(sent, to)
				return nil
			}), true
		}
		return nil, false
	})

	const ssoEmail = "sso-only@example.com"
	const pwEmail = "has-password@example.com"
	if err := db.Create(&models.User{Email: ssoEmail, PasswordHash: ""}).Error; err != nil {
		t.Fatalf("seed sso user: %v", err)
	}
	if err := db.Create(&models.User{Email: pwEmail, PasswordHash: "$2a$10$notarealhashbutnonempty0000000000000000000000000000"}).Error; err != nil {
		t.Fatalf("seed password user: %v", err)
	}

	if code, body := forgotFor(t, srv, ssoEmail); code != http.StatusOK {
		t.Fatalf("forgot for SSO account: got %d (%s), want 200", code, body)
	}

	var ssoUser models.User
	if err := db.Where("email = ?", ssoEmail).First(&ssoUser).Error; err != nil {
		t.Fatalf("reload sso user: %v", err)
	}
	if ssoUser.ResetTokenHash != "" {
		t.Fatalf("a reset token was issued for an SSO-only account: %q", ssoUser.ResetTokenHash)
	}
	if ssoUser.PasswordHash != "" {
		t.Fatalf("the SSO-only account acquired a password hash: %q", ssoUser.PasswordHash)
	}
	for _, to := range sent {
		if to == ssoEmail {
			t.Fatal("a reset link was mailed to an SSO-only account")
		}
	}

	// The ordinary path must still work — otherwise the guard above would pass
	// just as well with password reset deleted entirely.
	if code, body := forgotFor(t, srv, pwEmail); code != http.StatusOK {
		t.Fatalf("forgot for password account: got %d (%s), want 200", code, body)
	}
	var pwUser models.User
	if err := db.Where("email = ?", pwEmail).First(&pwUser).Error; err != nil {
		t.Fatalf("reload password user: %v", err)
	}
	if pwUser.ResetTokenHash == "" {
		t.Fatal("no reset token issued for an ordinary password account")
	}
	found := false
	for _, to := range sent {
		if to == pwEmail {
			found = true
		}
	}
	if !found {
		t.Fatalf("no reset link mailed to the password account; mails went to %v", sent)
	}
}

// TestForgotPasswordIsNotAnSSOOracle: the refusal must be indistinguishable
// from every other outcome to an unauthenticated caller. Three inputs — an
// SSO-only address, a password address, and an address with no account at all —
// must produce the same status and the same body.
func TestForgotPasswordIsNotAnSSOOracle(t *testing.T) {
	srv, db := newTestHandler(t)

	if err := db.Create(&models.User{Email: "oracle-sso@example.com", PasswordHash: ""}).Error; err != nil {
		t.Fatalf("seed sso user: %v", err)
	}
	if err := db.Create(&models.User{Email: "oracle-pw@example.com", PasswordHash: "$2a$10$notarealhashbutnonempty0000000000000000000000000000"}).Error; err != nil {
		t.Fatalf("seed password user: %v", err)
	}

	ssoCode, ssoBody := forgotFor(t, srv, "oracle-sso@example.com")
	pwCode, pwBody := forgotFor(t, srv, "oracle-pw@example.com")
	missCode, missBody := forgotFor(t, srv, "oracle-nobody@example.com")

	if ssoCode != pwCode || ssoCode != missCode {
		t.Fatalf("status leaks the account kind: sso=%d password=%d unknown=%d", ssoCode, pwCode, missCode)
	}
	if ssoBody != pwBody || ssoBody != missBody {
		t.Fatalf("body leaks the account kind: sso=%q password=%q unknown=%q", ssoBody, pwBody, missBody)
	}
}
