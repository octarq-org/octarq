package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// captureInviteMail installs a system mail sender on h that records the text
// body of every mail the handlers send, and returns the recording.
//
// Every invite test goes through here now: the raw invite token is delivered to
// the invited mailbox and nowhere else, so reading the mail is the only way to
// obtain it — which is exactly the property the tests should be exercising.
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

// tokenFromInviteMail pulls the raw token out of the accept link in the most
// recently mailed invite.
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

// inviteAndReadToken posts an invite for email and returns the raw token the
// invited mailbox received.
func inviteAndReadToken(t *testing.T, srv http.Handler, sent *[]string, cookies []*http.Cookie, email string) string {
	t.Helper()
	rec := do(srv, "POST", "/api/org/members", cookies, fmt.Sprintf(`{"email":%q,"role":"member"}`, email))
	if rec.Code != http.StatusOK {
		t.Fatalf("addOrgMember: got %d (%s)", rec.Code, rec.Body.String())
	}
	return tokenFromInviteMail(t, sent)
}

// TestInviteResponseNeverCarriesTheToken is the guard for the escalation this
// endpoint used to allow.
//
// Redeeming an invite token sets a password AND marks the address verified
// (acceptInvite), so returning the raw token to the INVITER handed them a
// working credential for a mailbox they do not control. On Cloud every
// self-serve signup is an org admin, which made that "invite the victim, keep
// the token, own the account" for anyone who can sign up — while the real owner
// gets a 409 on registration and a dead-ended SSO login.
//
// The response must therefore carry no invite material at all: not the token,
// not a URL containing it, not under any other key.
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

	// The token that actually exists, recovered from the mail the invitee got.
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
	// Delivery status is reported, so the UI can tell the operator to configure
	// mail rather than silently dropping invites.
	if v, ok := m["emailSent"].(bool); !ok || !v {
		t.Fatalf("expected emailSent=true, got %v", m["emailSent"])
	}
}

// TestInviteResponseOmitsTokenWhenMailUnconfigured pins the same rule for the
// case that used to justify returning the token: no mail sender mounted. The
// invite still succeeds (a 500 here would break inviting on a fresh instance),
// but the answer is emailSent=false, never the secret.
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
