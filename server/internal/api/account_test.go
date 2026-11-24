package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestAccountExportAndPurge(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(301)
	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	sess := sessionCookies(t, ownerUID, org)

	db.Create(&testLink{OrgID: org, Slug: "exp1", Target: "https://e.com"})
	db.Create(&models.Token{OrgID: org, Name: "t", Hash: models.HashToken("oct_acct_export_token_000000000000"), Prefix: "oct_acct"})
	db.Create(&testSMTPSender{OrgID: org, Name: "relay", Host: "smtp.x", FromEmail: "a@x.com", Pass: "supersecret"})

	rec := do(srv, "GET", "/api/account/export", sess, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	files := unzipTestArchive(t, rec.Body.Bytes())
	for _, key := range []string{"metadata.json", "audit_logs.json"} {
		if _, ok := files[key]; !ok {
			t.Errorf("export missing core file %q", key)
		}
	}
	for fName, fContent := range files {
		fStr := string(fContent)
		if containsAny(fStr, "supersecret", models.HashToken("oct_acct_export_token_000000000000")) {
			t.Errorf("export leaked a secret in %s (smtp pass or token hash)", fName)
		}
	}

	if rec := do(srv, "DELETE", "/api/account/data", sess, `{"confirm":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("purge w/o confirm: got %d, want 400", rec.Code)
	}

	if rec := do(srv, "DELETE", "/api/account/data", sess, `{"confirm":"DELETE MY DATA"}`); rec.Code != http.StatusOK {
		t.Fatalf("purge: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var n int64
	db.Model(&models.Token{}).Where("owner_id = ?", org).Count(&n)
	if n != 0 {
		t.Errorf("tokens remain after purge: %d", n)
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

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if jsonContains(s, sub) {
			return true
		}
	}
	return false
}

func jsonContains(haystack, needle string) bool {
	if idx := indexOf(haystack, needle); idx >= 0 {
		return true
	}
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
