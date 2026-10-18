package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/plugins/links"
)

func TestExportAccount_CompletenessAndSecretRedaction(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)

	const orgA uint = 301
	const userA uint = 3001
	const secretStr = "SECRET-DO-NOT-LEAK-WEBHOOK-KEY"

	seedOrgFullData(t, h, orgA, userA, secretStr)
	cookiesA := sessionCookies(t, userA, orgA)

	rec := do(srv, http.MethodGet, "/api/account/export", cookiesA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	files := unzipTestArchive(t, rec.Body.Bytes())

	// 1. Assert raw secret is NOT present in any export file
	for fName, fBytes := range files {
		fStr := string(fBytes)
		if contains(fStr, secretStr) {
			t.Errorf("SECRET LEAK: export file %s contains raw webhook secret %q", fName, secretStr)
		}
		if contains(fStr, "inbound-secret-token") {
			t.Errorf("SECRET LEAK: export file %s contains raw org inbound token", fName)
		}
	}

	// 2. Assert standard files are present in the zip
	for _, key := range []string{
		"metadata.json", "links.csv", "domains.json", "mailboxes.json", "audit_logs.json",
	} {
		if _, ok := files[key]; !ok {
			t.Errorf("missing file %q in export zip", key)
		}
	}

	// 3. Verify metadata.json counts and structure
	var meta map[string]any
	if err := json.Unmarshal(files["metadata.json"], &meta); err != nil {
		t.Fatalf("unmarshal metadata.json: %v", err)
	}
	if meta["orgId"] != float64(orgA) {
		t.Errorf("metadata orgId = %v, want %d", meta["orgId"], orgA)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && searchSubstr(s, substr)))
}

// TestExportAccount_IncludesPluginData guards the plugin.ExportFunc contract:
// the export must merge in every mounted plugin's data through
// LookupServiceAs[plugin.ExportFunc]. Short-circuiting that guard leaves the
// plugin's key out of the export and reds this test.
func TestExportAccount_IncludesPluginData(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 401
	const userA uint = 4001

	seedOrgFullData(t, h, orgA, userA, "SECRET-ORG-A")
	// A link row only the links plugin's export service can contribute — the
	// core export has no idea plugin tables exist.
	if err := db.Create(&links.Link{OrgID: orgA, Slug: "guard-export", Target: "https://example.com/guard"}).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}
	cookies := sessionCookies(t, userA, orgA)

	rec := do(srv, http.MethodGet, "/api/account/export", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	files := unzipTestArchive(t, rec.Body.Bytes())
	linksPluginData, ok := files["plugins/links.json"]
	if !ok {
		t.Fatalf("export missing plugins/links.json: got files %+v", files)
	}

	var res map[string]any
	if err := json.Unmarshal(linksPluginData, &res); err != nil {
		t.Fatalf("unmarshal plugins/links.json: %v", err)
	}
	linksArr, ok := res["links"].([]any)
	if !ok || len(linksArr) == 0 {
		t.Errorf("export missing the links plugin's data (plugin.ExportFunc guard broken?): %+v", res)
	}
	linksCSV := string(files["links.csv"])
	if !contains(linksCSV, "guard-export") {
		t.Errorf("links.csv missing guard-export link: %s", linksCSV)
	}
}

func searchSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
