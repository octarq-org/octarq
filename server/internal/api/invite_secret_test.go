package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func captureInviteMail(t *testing.T, h *Handler) *[]string {
	t.Helper()
	var sent []string
	send := plugin.SystemMailSender(func(to, subject, htmlBody, textBody string) error {
		sent = append(sent, textBody)
		return nil
	})
	h.SetServiceLookup(func(name string) (any, bool) {
		if name == plugin.ServiceMailSendSystem {
			return send, true
		}
		return nil, false
	})
	return &sent
}

func tokenFromInviteMail(t *testing.T, sent *[]string) string {
	t.Helper()
	if len(*sent) == 0 {
		t.Fatal("no invite mail was sent")
	}
	body := (*sent)[len(*sent)-1]
	_, after, ok := strings.Cut(body, "token=")
	if !ok {
		t.Fatalf("no accept link in invite mail: %q", body)
	}
	return strings.TrimSpace(strings.Fields(after)[0])
}

func inviteAndReadToken(t *testing.T, srv http.Handler, sent *[]string, cookies []*http.Cookie, email string) string {
	t.Helper()
	rec := do(srv, "POST", "/api/org/members", cookies, fmt.Sprintf(`{"email":%q,"role":"member"}`, email))
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember: got %d (%s)", rec.Code, rec.Body.String())
	}
	return tokenFromInviteMail(t, sent)
}

func TestInviteResponseNeverCarriesTheToken(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "inviter@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)
	sent := captureInviteMail(t, h)

	email := t.Name() + "+victim@example.com"
	rec := do(srv, "POST", "/api/org/members", adminSession, fmt.Sprintf(`{"email":%q,"role":"member"}`, email))
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember: got %d (%s)", rec.Code, rec.Body.String())
	}

	raw := tokenFromInviteMail(t, sent)
	if raw == "" {
		t.Fatal("no token in the invite mail — test cannot prove anything")
	}

	body := rec.Body.String()
	if strings.Contains(body, raw) {
		t.Fatalf("invite response leaked the raw invite token: %s", body)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	for _, k := range []string{"inviteToken", "inviteUrl", "token", "acceptUrl"} {
		if v, ok := m[k]; ok {
			t.Fatalf("invite response still carries %q = %v", k, v)
		}
	}
	if v, ok := m["emailSent"].(bool); !ok || !v {
		t.Fatalf("expected emailSent=true, got %v", m["emailSent"])
	}
}

func TestInviteResponseOmitsTokenWhenMailUnconfigured(t *testing.T) {
	srv, db := newTestHandler(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "nomail@example.com", "owner")
	adminSession := sessionCookies(t, adminUID, orgID)

	rec := do(srv, "POST", "/api/org/members", adminSession,
		fmt.Sprintf(`{"email":%q,"role":"member"}`, t.Name()+"+invitee@example.com"))
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember: got %d (%s)", rec.Code, rec.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	if _, ok := m["inviteToken"]; ok {
		t.Fatalf("invite response carries inviteToken with no mail configured: %s", rec.Body.String())
	}
	if _, ok := m["inviteUrl"]; ok {
		t.Fatalf("invite response carries inviteUrl with no mail configured: %s", rec.Body.String())
	}
	if v, ok := m["emailSent"].(bool); !ok || v {
		t.Fatalf("expected emailSent=false with no sender mounted, got %v", m["emailSent"])
	}
}

func TestResendOrgMemberInvite(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	const orgID = uint(1)
	adminUID := seedOrgMember(t, db, orgID, "admin_resend@example.com", "admin")
	adminSession := sessionCookies(t, adminUID, orgID)
	memberUID := seedOrgMember(t, db, orgID, "regular_resend@example.com", "member")
	memberSession := sessionCookies(t, memberUID, orgID)
	sent := captureInviteMail(t, h)

	email := "pending_invitee@example.com"
	rec := do(srv, "POST", "/api/org/members", adminSession, fmt.Sprintf(`{"email":%q,"role":"member"}`, email))
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember: got %d (%s)", rec.Code, rec.Body.String())
	}
	var invitedUser models.User
	if err := db.Where("email = ?", email).First(&invitedUser).Error; err != nil {
		t.Fatalf("find invited user: %v", err)
	}
	initialToken := tokenFromInviteMail(t, sent)
	if initialToken == "" {
		t.Fatal("expected initial invite token in email")
	}

	rec = do(srv, "POST", fmt.Sprintf("/api/org/members/%d/resend", invitedUser.ID), memberSession, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member resend, got %d", rec.Code)
	}

	rec = do(srv, "POST", "/api/org/members/99999/resend", adminSession, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent user resend, got %d", rec.Code)
	}

	h.resendInviteLimits.Delete(uint64(invitedUser.ID))
	rec = do(srv, "POST", fmt.Sprintf("/api/org/members/%d/resend", invitedUser.ID), adminSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid resend, got %d (%s)", rec.Code, rec.Body.String())
	}
	newToken := tokenFromInviteMail(t, sent)
	if newToken == "" || newToken == initialToken {
		t.Fatalf("expected newly generated token, got initial=%q, new=%q", initialToken, newToken)
	}

	rec = do(srv, "POST", fmt.Sprintf("/api/org/members/%d/resend", invitedUser.ID), adminSession, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for rapid resend, got %d", rec.Code)
	}

	var userRecord models.User
	db.First(&userRecord, invitedUser.ID)
	userRecord.InviteTokenHash = ""
	db.Save(&userRecord)
	h.resendInviteLimits.Delete(uint64(invitedUser.ID))
	rec = do(srv, "POST", fmt.Sprintf("/api/org/members/%d/resend", invitedUser.ID), adminSession, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for already accepted member resend, got %d", rec.Code)
	}
}
