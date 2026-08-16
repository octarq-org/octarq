package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/links"
)

func seedOrgFullData(t *testing.T, h *Handler, orgID, userID uint, webhookSecret string) {
	t.Helper()
	db := h.db

	org := models.Org{
		ID:           orgID,
		Name:         "Test Org",
		Slug:         "test-org-" + string(rune('a'+orgID)),
		InboundToken: "inbound-secret-token-" + string(rune('a'+orgID)),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}

	user := models.User{
		ID:        userID,
		Email:     "user-" + string(rune('a'+userID)) + "@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	member := models.OrgMember{
		OrgID:  orgID,
		UserID: userID,
		Role:   "owner",
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}

	token := models.Token{
		OrgID:     orgID,
		Name:      "test-token",
		Hash:      "hash-" + string(rune('a'+orgID)),
		Prefix:    "oct_test",
		CreatedAt: time.Now(),
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}

	channel := models.NotificationChannel{
		OrgID:     orgID,
		Name:      "test-channel",
		Type:      "webhook",
		Config:    `{"url":"https://example.com/hook","secret":"bot-secret-123"}`,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	wsSetting := models.WorkspaceSetting{
		OrgID: orgID,
		Key:   "theme",
		Value: "dark",
	}
	if err := db.Create(&wsSetting).Error; err != nil {
		t.Fatalf("seed workspace setting: %v", err)
	}

	pluginSetting := models.PluginSetting{
		OrgID:     orgID,
		Plugin:    "infra",
		Enabled:   true,
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&pluginSetting).Error; err != nil {
		t.Fatalf("seed plugin setting: %v", err)
	}

	auditLog := models.AuditLog{
		OrgID:      orgID,
		ActorID:    userID,
		Action:     "test.action",
		TargetType: "test",
		TargetID:   1,
		IP:         "127.0.0.1",
		CreatedAt:  time.Now(),
	}
	if err := db.Create(&auditLog).Error; err != nil {
		t.Fatalf("seed audit log: %v", err)
	}

	webhook := models.Webhook{
		OrgID:     orgID,
		Name:      "test-webhook",
		URL:       "https://example.com/webhook",
		Secret:    webhookSecret,
		Events:    "*",
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&webhook).Error; err != nil {
		t.Fatalf("seed webhook: %v", err)
	}

	abuseReport := models.AbuseReport{
		OrgID:       orgID,
		Slug:        "bad-slug",
		Target:      "https://phishing.example.com",
		Reason:      "phishing",
		Description: "phishing report",
		ReporterIP:  "10.0.0.1",
		Status:      "open",
		CreatedAt:   time.Now(),
	}
	if err := db.Create(&abuseReport).Error; err != nil {
		t.Fatalf("seed abuse report: %v", err)
	}

	session := models.Session{
		UserID:     userID,
		OrgID:      orgID,
		Token:      "session-token-hash-" + string(rune('a'+orgID)),
		IP:         "127.0.0.1",
		UserAgent:  "test-agent",
		LastSeenAt: time.Now(),
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		CreatedAt:  time.Now(),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Per-workspace rows Pro plugins keep in the shared settings table, namespaced
	// "org_<id>." — billing's and finance's Stripe secrets, ai's LLM credentials.
	// Nothing else in the purge reaches these, so they are seeded here alongside an
	// instance-level row that must survive.
	settings := []models.Setting{
		{Key: fmt.Sprintf("org_%d.pay.stripe.secret_key", orgID), Value: "sealed:" + webhookSecret},
		{Key: fmt.Sprintf("org_%d.pay.stripe.webhook_secret", orgID), Value: "sealed:whsec"},
		{Key: fmt.Sprintf("org_%d.ai.llm.provider_id", orgID), Value: "7"},
	}
	for _, s := range settings {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("seed setting %q: %v", s.Key, err)
		}
	}
	// An instance-level row the sweep must not touch. Created once; the settings
	// table is shared across the orgs a test seeds.
	db.Where(models.Setting{Key: instanceSettingKey}).
		Attrs(models.Setting{Value: "Instance"}).
		FirstOrCreate(&models.Setting{})
}

// instanceSettingKey is a settings row that belongs to the instance, not to any
// workspace — the control for the prefix-scoped purge sweep.
const instanceSettingKey = "app_name"

func TestPurgeAccount_DeletesEverythingForOrg(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 101
	const userA uint = 1001

	seedOrgFullData(t, h, orgA, userA, "SECRET-ORG-A")
	cookies := sessionCookies(t, userA, orgA)

	rec := do(srv, http.MethodDelete, "/api/account/data", cookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	// Assert every single table for Org A has 0 rows
	var (
		tokenCount   int64
		channelCount int64
		wsCount      int64
		pluginCount  int64
		auditCount   int64
		webhookCount int64
		abuseCount   int64
		sessionCount int64
		memberCount  int64
		orgCount     int64
	)

	db.Model(&models.Token{}).Where("owner_id = ?", orgA).Count(&tokenCount)
	db.Model(&models.NotificationChannel{}).Where("owner_id = ?", orgA).Count(&channelCount)
	db.Model(&models.WorkspaceSetting{}).Where("org_id = ?", orgA).Count(&wsCount)
	db.Model(&models.PluginSetting{}).Where("org_id = ?", orgA).Count(&pluginCount)
	db.Model(&models.AuditLog{}).Where("org_id = ?", orgA).Count(&auditCount)
	db.Model(&models.Webhook{}).Where("owner_id = ?", orgA).Count(&webhookCount)
	db.Model(&models.AbuseReport{}).Where("owner_id = ?", orgA).Count(&abuseCount)
	db.Model(&models.Session{}).Where("org_id = ?", orgA).Count(&sessionCount)
	db.Model(&models.OrgMember{}).Where("org_id = ?", orgA).Count(&memberCount)
	db.Model(&models.Org{}).Where("id = ?", orgA).Count(&orgCount)

	if tokenCount != 0 {
		t.Errorf("Token count for Org A = %d, want 0", tokenCount)
	}
	if channelCount != 0 {
		t.Errorf("NotificationChannel count for Org A = %d, want 0", channelCount)
	}
	if wsCount != 0 {
		t.Errorf("WorkspaceSetting count for Org A = %d, want 0", wsCount)
	}
	if pluginCount != 0 {
		t.Errorf("PluginSetting count for Org A = %d, want 0", pluginCount)
	}
	if auditCount != 0 {
		t.Errorf("AuditLog count for Org A = %d, want 0", auditCount)
	}
	if webhookCount != 0 {
		t.Errorf("Webhook count for Org A = %d, want 0", webhookCount)
	}
	if abuseCount != 0 {
		t.Errorf("AbuseReport count for Org A = %d, want 0", abuseCount)
	}
	if sessionCount != 0 {
		t.Errorf("Session count for Org A = %d, want 0", sessionCount)
	}
	if memberCount != 0 {
		t.Errorf("OrgMember count for Org A = %d, want 0", memberCount)
	}
	if orgCount != 0 {
		t.Errorf("Org count for Org A = %d, want 0", orgCount)
	}

	// The workspace's namespaced settings rows — its Stripe secret, webhook secret
	// and LLM credentials. These live in the shared settings table and no plugin
	// table owns them, so before the sweep they outlived the workspace entirely and
	// a recycled org id would have inherited them.
	var orgSettings int64
	db.Model(&models.Setting{}).Where("key LIKE ?", fmt.Sprintf("org_%d.%%", orgA)).Count(&orgSettings)
	if orgSettings != 0 {
		t.Errorf("namespaced settings for Org A = %d, want 0 — the workspace's secrets survived its deletion", orgSettings)
	}

	// The instance-level setting seeded before the purge must be untouched: the
	// sweep is prefix-scoped, not a blanket delete on the settings table.
	var instance int64
	db.Model(&models.Setting{}).Where("key = ?", instanceSettingKey).Count(&instance)
	if instance != 1 {
		t.Errorf("instance-level setting count = %d, want 1 — the sweep is too wide", instance)
	}
}

func TestPurgeAccount_DoesNotAffectNeighborOrg(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 201
	const userA uint = 2001

	const orgB uint = 202
	const userB uint = 2002

	seedOrgFullData(t, h, orgA, userA, "SECRET-ORG-A")
	seedOrgFullData(t, h, orgB, userB, "SECRET-ORG-B")

	cookiesA := sessionCookies(t, userA, orgA)

	rec := do(srv, http.MethodDelete, "/api/account/data", cookiesA, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge request for Org A failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	// Assert every table for Org B still has its row intact (1 row)
	var (
		tokenCount   int64
		channelCount int64
		wsCount      int64
		pluginCount  int64
		auditCount   int64
		webhookCount int64
		abuseCount   int64
		sessionCount int64
		memberCount  int64
		orgCount     int64
	)

	db.Model(&models.Token{}).Where("owner_id = ?", orgB).Count(&tokenCount)
	db.Model(&models.NotificationChannel{}).Where("owner_id = ?", orgB).Count(&channelCount)
	db.Model(&models.WorkspaceSetting{}).Where("org_id = ?", orgB).Count(&wsCount)
	db.Model(&models.PluginSetting{}).Where("org_id = ?", orgB).Count(&pluginCount)
	db.Model(&models.AuditLog{}).Where("org_id = ?", orgB).Count(&auditCount)
	db.Model(&models.Webhook{}).Where("owner_id = ?", orgB).Count(&webhookCount)
	db.Model(&models.AbuseReport{}).Where("owner_id = ?", orgB).Count(&abuseCount)
	db.Model(&models.Session{}).Where("org_id = ?", orgB).Count(&sessionCount)
	db.Model(&models.OrgMember{}).Where("org_id = ?", orgB).Count(&memberCount)
	db.Model(&models.Org{}).Where("id = ?", orgB).Count(&orgCount)

	if tokenCount != 1 {
		t.Errorf("NEIGHBOR LEAK: Token count for Org B = %d, want 1", tokenCount)
	}
	if channelCount != 1 {
		t.Errorf("NEIGHBOR LEAK: NotificationChannel count for Org B = %d, want 1", channelCount)
	}
	if wsCount != 1 {
		t.Errorf("NEIGHBOR LEAK: WorkspaceSetting count for Org B = %d, want 1", wsCount)
	}
	if pluginCount != 1 {
		t.Errorf("NEIGHBOR LEAK: PluginSetting count for Org B = %d, want 1", pluginCount)
	}
	if auditCount != 1 {
		t.Errorf("NEIGHBOR LEAK: AuditLog count for Org B = %d, want 1", auditCount)
	}
	if webhookCount != 1 {
		t.Errorf("NEIGHBOR LEAK: Webhook count for Org B = %d, want 1", webhookCount)
	}
	if abuseCount != 1 {
		t.Errorf("NEIGHBOR LEAK: AbuseReport count for Org B = %d, want 1", abuseCount)
	}
	if sessionCount != 1 {
		t.Errorf("NEIGHBOR LEAK: Session count for Org B = %d, want 1", sessionCount)
	}
	if memberCount != 1 {
		t.Errorf("NEIGHBOR LEAK: OrgMember count for Org B = %d, want 1", memberCount)
	}
	if orgCount != 1 {
		t.Errorf("NEIGHBOR LEAK: Org count for Org B = %d, want 1", orgCount)
	}
}

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

// TestPurgeAccount_InvokesPluginPurgeServices guards the plugin.PurgeFunc
// contract: the purge must reach every mounted plugin's purge service through
// LookupServiceAs[plugin.PurgeFunc]. The core purge transaction never touches
// plugin tables, so a link row surviving the purge means the plugin service
// never ran. Short-circuiting the guard leaves the row behind and reds this
// test.
func TestPurgeAccount_InvokesPluginPurgeServices(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	const orgA uint = 501
	const userA uint = 5001

	seedOrgFullData(t, h, orgA, userA, "SECRET-ORG-A")
	if err := db.Create(&links.Link{OrgID: orgA, Slug: "guard-purge", Target: "https://example.com/guard"}).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}
	cookies := sessionCookies(t, userA, orgA)

	rec := do(srv, http.MethodDelete, "/api/account/data", cookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	var linkCount int64
	db.Model(&links.Link{}).Where("owner_id = ?", orgA).Count(&linkCount)
	if linkCount != 0 {
		t.Errorf("links rows for Org A = %d, want 0 — the links purge service never ran (plugin.PurgeFunc guard broken?)", linkCount)
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
