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

	bodyStr := rec.Body.String()

	// 1. Assert raw secret is NOT present in export body
	if contains(bodyStr, secretStr) {
		t.Errorf("SECRET LEAK: export contains raw webhook secret %q", secretStr)
	}
	if contains(bodyStr, "inbound-secret-token") {
		t.Errorf("SECRET LEAK: export contains raw org inbound token")
	}

	// 2. Unmarshal and assert struct components are present
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal export json: %v", err)
	}

	for _, key := range []string{
		"organization", "apiTokens", "notificationChannels",
		"workspaceSettings", "pluginSettings", "auditLogs",
		"webhooks", "abuseReports",
	} {
		if _, ok := res[key]; !ok {
			t.Errorf("missing key %q in export response: %+v", key, res)
		}
	}

	// 3. Verify webhook secret is redacted
	webhooksArr, _ := res["webhooks"].([]any)
	if len(webhooksArr) != 1 {
		t.Fatalf("expected 1 webhook in export, got %d", len(webhooksArr))
	}
	whMap, _ := webhooksArr[0].(map[string]any)
	if sec, _ := whMap["secret"].(string); sec != "[redacted]" {
		t.Errorf("webhook secret = %q, want [redacted]", sec)
	}

	// 4. Verify notification channel config is redacted
	channelsArr, _ := res["notificationChannels"].([]any)
	if len(channelsArr) != 1 {
		t.Fatalf("expected 1 channel in export, got %d", len(channelsArr))
	}
	chMap, _ := channelsArr[0].(map[string]any)
	if cfg, _ := chMap["config"].(string); cfg != "[redacted]" {
		t.Errorf("notification channel config = %q, want [redacted]", cfg)
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

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal export json: %v", err)
	}
	linksArr, ok := res["links"].([]any)
	if !ok || len(linksArr) != 1 {
		t.Errorf("export missing the links plugin's data (plugin.ExportFunc guard broken?): %+v", res)
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
