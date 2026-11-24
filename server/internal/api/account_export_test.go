package api

import (
	"encoding/json"
	"net/http"
	"testing"
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

	for fName, fBytes := range files {
		fStr := string(fBytes)
		if contains(fStr, secretStr) {
			t.Errorf("SECRET LEAK: export file %s contains raw webhook secret %q", fName, secretStr)
		}
		if contains(fStr, "inbound-secret-token") {
			t.Errorf("SECRET LEAK: export file %s contains raw org inbound token", fName)
		}
	}

	for _, key := range []string{"metadata.json", "audit_logs.json"} {
		if _, ok := files[key]; !ok {
			t.Errorf("missing file %q in export zip", key)
		}
	}
	for _, forbidden := range []string{"links.csv", "domains.json", "mailboxes.json"} {
		if _, ok := files[forbidden]; ok {
			t.Errorf("core export must not contain business file %q", forbidden)
		}
	}

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

func searchSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
