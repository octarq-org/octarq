package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestAccountExportAndPurge(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(301)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	sess := sessionCookies(t, ownerUID, org)

	// Seed a couple of org-owned records + a secret-bearing one.
	db.Create(&models.Link{OrgID: org, Slug: "exp1", Target: "https://e.com"})
	db.Create(&models.Token{OrgID: org, Name: "t", Hash: models.HashToken("led_acct_export_token_000000000000"), Prefix: "led_acct"})
	db.Create(&models.SMTPSender{OrgID: org, Name: "relay", Host: "smtp.x", FromEmail: "a@x.com", Pass: "supersecret"})

	// Export must include data but never leak the secrets.
	rec := do(srv, "GET", "/api/account/export", sess, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body, `"exp1"`, `"relay"`) {
		t.Errorf("export missing expected records: %s", body)
	}
	if containsAny(body, "supersecret", models.HashToken("led_acct_export_token_000000000000")) {
		t.Error("export leaked a secret (smtp pass or token hash)")
	}

	// Purge without confirmation is rejected.
	if rec := do(srv, "DELETE", "/api/account/data", sess, `{"confirm":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("purge w/o confirm: got %d, want 400", rec.Code)
	}

	// Purge with confirmation wipes the org's rows.
	if rec := do(srv, "DELETE", "/api/account/data", sess, `{"confirm":"DELETE MY DATA"}`); rec.Code != http.StatusOK {
		t.Fatalf("purge: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var n int64
	db.Model(&models.Link{}).Where("owner_id = ?", org).Count(&n)
	if n != 0 {
		t.Errorf("links remain after purge: %d", n)
	}
}

func TestAccountExportRequiresOwnerAdmin(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(302)
	seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	sess := sessionCookies(t, memberUID, org)

	if rec := do(srv, "GET", "/api/account/export", sess, ""); rec.Code != http.StatusForbidden {
		t.Errorf("member export: got %d, want 403", rec.Code)
	}
	if rec := do(srv, "DELETE", "/api/account/data", sess, `{"confirm":"DELETE MY DATA"}`); rec.Code != http.StatusForbidden {
		t.Errorf("member purge: got %d, want 403", rec.Code)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !jsonContains(s, sub) {
			return false
		}
	}
	return true
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if jsonContains(s, sub) {
			return true
		}
	}
	return false
}

// jsonContains is a substring check that also tolerates JSON escaping.
func jsonContains(haystack, needle string) bool {
	if idx := indexOf(haystack, needle); idx >= 0 {
		return true
	}
	// Try the JSON-escaped form (e.g. for values with special chars).
	if b, err := json.Marshal(needle); err == nil {
		trimmed := string(b[1 : len(b)-1])
		return indexOf(haystack, trimmed) >= 0
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
