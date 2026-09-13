package api

// Guard tests for the security-audit fixes (F-seclow). Each test pins one
// product-code guard and is designed so that short-circuiting the guard (e.g.
// `if false && cond`) turns it red — see F-seclow-report.md for the mutation
// trace.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

// Guard 1 (L-1): the raw invite token must never be stored — the DB keeps only
// its SHA-256 hash — yet the raw token shown to the operator must still redeem
// the invite.
func TestInviteTokenStoredHashedAndRawStillAccepts(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "invitemgr@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)
	sent := captureInviteMail(t, h)

	email := t.Name() + "+guest@example.com"
	rawToken := inviteAndReadToken(t, srv, sent, adminSession, email)
	if rawToken == "" {
		t.Fatal("expected a raw invite token in the mailed link")
	}

	var user models.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		t.Fatalf("invited user not found: %v", err)
	}
	if user.InviteTokenHash == "" {
		t.Fatal("no invite token hash stored in the DB")
	}
	if user.InviteTokenHash == rawToken {
		t.Fatal("invite token stored in plaintext: DB value equals the raw token")
	}
	if user.InviteTokenHash != hashToken(rawToken) {
		t.Fatalf("stored hash %q is not the SHA-256 of the raw token", user.InviteTokenHash)
	}

	// The raw token from the mailed link must still redeem the invite.
	rec := do(srv, "POST", "/api/auth/invite/accept", nil, fmt.Sprintf(`{"token":%q,"password":"teampass123"}`, rawToken))
	if rec.Code != http.StatusOK {
		t.Fatalf("accept with raw token: got %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var after models.User
	db.First(&after, user.ID)
	if after.InviteTokenHash != "" {
		t.Fatal("invite token hash not cleared on accept")
	}
	if after.PasswordHash == "" {
		t.Fatal("password hash not set on accept")
	}
}

// Guard 2 (L-3): spamming the public reset-completion endpoint from one IP must
// trip the recovery budget and answer 429 instead of processing forever.
func TestResetPasswordRateLimited(t *testing.T) {
	srv, _ := newTestHandler(t)
	body := `{"token":"garbage","password":"newpassword123"}`

	for i := 0; i < 5; i++ {
		rec := do(srv, "POST", "/api/auth/reset", nil, body)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d: hit 429 before the budget was spent", i+1)
		}
	}
	rec6 := do(srv, "POST", "/api/auth/reset", nil, body)
	if rec6.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 6: got %d (%s), want 429", rec6.Code, rec6.Body.String())
	}
}

// Guard 3 (L-8): a successful password change must land in audit_logs, and the
// record must never carry the password — old or new.
func TestChangePasswordAuditsWithoutStoringPassword(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := registerUser(t, srv, db, "owner@example.com", "originalpw1")

	var before models.User
	if err := db.Where("email = ?", "owner@example.com").First(&before).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/password", cookies,
		`{"currentPassword":"originalpw1","newPassword":"replacementpw2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change password: got %d (%s)", rec.Code, rec.Body.String())
	}

	log := waitForAudit(t, db, "user.change_password")
	if log.ActorID != before.ID {
		t.Fatalf("change-password audit actor: got %d, want %d", log.ActorID, before.ID)
	}
	if strings.Contains(log.Meta, "originalpw1") || strings.Contains(log.Meta, "replacementpw2") {
		t.Fatalf("change-password audit record leaks a password: %s", log.Meta)
	}
}
