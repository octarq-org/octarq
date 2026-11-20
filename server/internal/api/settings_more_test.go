package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestBrandAndAppName(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	if name := h.AppName(); name != "octarq" {
		t.Errorf("AppName() = %q, want octarq", name)
	}
	logo, c1, c2 := h.Brand()
	if logo != "" || c1 != "" || c2 != "" {
		t.Errorf("Brand() = %q, %q, %q, want empty", logo, c1, c2)
	}

	db.Save(&models.Setting{Key: keyAppName, Value: "Custom Octarq"})
	db.Save(&models.Setting{Key: keyBrandLogo, Value: "https://example.com/logo.png"})
	db.Save(&models.Setting{Key: keyBrandColor, Value: "#112233"})
	db.Save(&models.Setting{Key: keyBrandColor2, Value: "#445566"})

	if name := h.AppName(); name != "Custom Octarq" {
		t.Errorf("AppName() = %q, want Custom Octarq", name)
	}
	logo, c1, c2 = h.Brand()
	if logo != "https://example.com/logo.png" || c1 != "#112233" || c2 != "#445566" {
		t.Errorf("Brand() = (%q, %q, %q)", logo, c1, c2)
	}

	h.SetWorkspaceSetting(10, keyAppName, "Workspace 10 App")
	h.SetWorkspaceSetting(10, keyBrandLogo, "https://example.com/org10.png")
	if name := h.AppNameFor(10); name != "Workspace 10 App" {
		t.Errorf("AppNameFor(10) = %q, want Workspace 10 App", name)
	}
	logo, c1, c2 = h.BrandFor(10)
	if logo != "https://example.com/org10.png" || c1 != "#112233" {
		t.Errorf("BrandFor(10) = (%q, %q, %q)", logo, c1, c2)
	}

	if err := h.SetGlobalSetting("test_key", "test_val"); err != nil {
		t.Fatalf("SetGlobalSetting: %v", err)
	}
	if val := h.GetGlobalSetting("test_key"); val != "test_val" {
		t.Errorf("GetGlobalSetting = %q, want test_val", val)
	}

	if !h.RequireEmailVerification() {
		t.Errorf("RequireEmailVerification() = false, want true")
	}
	req := httptest.NewRequest("GET", "/", nil)
	if h.IsInstanceAdmin(req) {
		t.Errorf("IsInstanceAdmin(unauthed) = true, want false")
	}
}

func TestMetricsTokenAndRateLimits(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	if tok := h.MetricsToken(); tok != "" {
		t.Errorf("MetricsToken() = %q, want empty", tok)
	}

	if err := h.setInstanceSecret(context.Background(), vaultKeyMetricsToken, "Metrics Bearer Token", "token", "my-secret-metrics-token", 1); err != nil {
		t.Fatalf("setInstanceSecret metrics token: %v", err)
	}
	if tok := h.MetricsToken(); tok != "my-secret-metrics-token" {
		t.Errorf("MetricsToken() = %q, want my-secret-metrics-token", tok)
	}

	authRPM, apiRPM, redirRPM := h.RateLimits()
	if authRPM != defaultAuthRPM || apiRPM != defaultAPIRPM || redirRPM != defaultRedirectRPM {
		t.Errorf("RateLimits() = (%d, %d, %d)", authRPM, apiRPM, redirRPM)
	}

	db.Save(&models.Setting{Key: keyRatelimitAuthRPM, Value: "120"})
	db.Save(&models.Setting{Key: keyRatelimitAPIRPM, Value: "600"})
	db.Save(&models.Setting{Key: keyRatelimitRedirRPM, Value: "1200"})
	authRPM, apiRPM, redirRPM = h.RateLimits()
	if authRPM != 120 || apiRPM != 600 || redirRPM != 1200 {
		t.Errorf("custom RateLimits() = (%d, %d, %d)", authRPM, apiRPM, redirRPM)
	}

	db.Save(&models.Setting{Key: keyRatelimitAuthRPM, Value: "not-a-number"})
	if n := h.settingInt(keyRatelimitAuthRPM, 55); n != 55 {
		t.Errorf("settingInt on invalid string = %d, want 55", n)
	}
}

func TestInstanceAdminCheck(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	nonAdminUser := models.User{Email: "member@example.com", IsInstanceAdmin: false}
	db.Create(&nonAdminUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: nonAdminUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, nonAdminUser.ID, 1)

	rec := do(srv, "GET", "/api/instance-settings", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin GET /api/instance-settings: got %d, want 403", rec.Code)
	}

	rec = do(srv, "GET", "/api/instance-settings", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET /api/instance-settings: got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestUpdateSettingsAndInboundToken(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	regularUser := models.User{Email: "regular@example.com"}
	db.Create(&regularUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: regularUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, regularUser.ID, 1)

	rec := do(srv, "GET", "/api/settings/inbound-token", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member GET inbound-token: got %d, want 403", rec.Code)
	}

	rec = do(srv, "PUT", "/api/settings", memberCookies, `{"reservedMailboxes":"admin,support"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member PUT /api/settings: got %d, want 403", rec.Code)
	}

	rec = do(srv, "PUT", "/api/settings", adminCookies, `{
		"reservedMailboxes": "admin, billing, support",
		"inboundToken": "custom-inbound-secret",
		"catchAll": true,
		"autoWrapLinks": true
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin PUT /api/settings: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(srv, "GET", "/api/settings/inbound-token", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET inbound-token: got %d (%s)", rec.Code, rec.Body.String())
	}
	var inbOut GetInboundTokenOutputBody
	if err := json.Unmarshal(rec.Body.Bytes(), &inbOut); err != nil {
		t.Fatalf("unmarshal inbound token: %v", err)
	}
	if inbOut.InboundToken != "custom-inbound-secret" {
		t.Errorf("inbound token: got %q, want custom-inbound-secret", inbOut.InboundToken)
	}

	rec = do(srv, "PUT", "/api/settings", adminCookies, `{"inboundToken":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate inbound token: got %d", rec.Code)
	}
	rec = do(srv, "GET", "/api/settings/inbound-token", adminCookies, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &inbOut)
	if inbOut.InboundToken == "" || inbOut.InboundToken == "custom-inbound-secret" {
		t.Errorf("expected rotated uuid token, got %q", inbOut.InboundToken)
	}

	rec = do(srv, "PUT", "/api/settings", adminCookies, `{"catchAll":false,"autoWrapLinks":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("turn off catchAll: got %d", rec.Code)
	}
	rec = do(srv, "GET", "/api/settings", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/settings: got %d", rec.Code)
	}
}

func TestUpdateInstanceSettingsAllFields(t *testing.T) {
	srv, _ := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	body := `{
		"reservedSlugs": "billing, support, status",
		"googleClientId": "google-client-id-123",
		"googleClientSecret": "google-secret-456",
		"githubClientId": "gh-client-id-123",
		"githubClientSecret": "gh-secret-456",
		"dataRetentionDays": 60,
		"allowRegistration": false,
		"requireEmailVerification": true,
		"appName": "Octarq Cloud",
		"baseDomain": "octarq.io",
		"sharedHosts": "app.octarq.io, console.octarq.io",
		"metricsToken": "metrics-bearer-secret",
		"ratelimitAuthRpm": 80,
		"ratelimitApiRpm": 500,
		"ratelimitRedirectRpm": 2000,
		"publicCorsOrigins": "https://dashboard.octarq.io",
		"systemSenderId": 1
	}`
	rec := do(srv, "PUT", "/api/instance-settings", adminCookies, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/instance-settings: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(srv, "GET", "/api/instance-settings", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/instance-settings: got %d", rec.Code)
	}
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal instance settings: %v", err)
	}
	if res["appName"] != "Octarq Cloud" {
		t.Errorf("appName = %v, want Octarq Cloud", res["appName"])
	}
	if res["sharedHosts"] != "app.octarq.io\nconsole.octarq.io" {
		t.Errorf("sharedHosts = %v, want app.octarq.io\\nconsole.octarq.io", res["sharedHosts"])
	}
	if res["allowRegistration"] != false {
		t.Errorf("allowRegistration = %v, want false", res["allowRegistration"])
	}
	if res["dataRetentionDays"] != float64(60) {
		t.Errorf("dataRetentionDays = %v, want 60", res["dataRetentionDays"])
	}
	if res["googleClientSecretSet"] != true || res["githubClientSecretSet"] != true || res["metricsTokenSet"] != true {
		t.Errorf("secrets flags mismatch: %+v", res)
	}

	clearBody := `{
		"googleClientSecret": "",
		"githubClientSecret": "",
		"metricsToken": "",
		"systemSenderId": 0,
		"allowRegistration": true,
		"requireEmailVerification": false
	}`
	rec = do(srv, "PUT", "/api/instance-settings", adminCookies, clearBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear secrets: got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = do(srv, "GET", "/api/instance-settings", adminCookies, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["googleClientSecretSet"] != false || res["githubClientSecretSet"] != false || res["metricsTokenSet"] != false {
		t.Errorf("expected secrets cleared, got: %+v", res)
	}
}

func TestDownloadBackup(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	nonAdminUser := models.User{Email: "user@example.com"}
	db.Create(&nonAdminUser)
	userCookies := sessionCookies(t, nonAdminUser.ID, 1)

	rec := do(srv, "GET", "/api/admin/backup", userCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin backup: got %d, want 403", rec.Code)
	}

	req := httptest.NewRequest("GET", "/api/admin/backup", nil)
	for _, c := range adminCookies {
		req.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("admin backup: got %d (%s)", rec2.Code, rec2.Body.String())
	}
	if disp := rec2.Header().Get("Content-Disposition"); disp == "" {
		t.Errorf("missing Content-Disposition header in backup response")
	}
}

func TestSettingsNilCtx(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)
	ctx := context.Background()

	if _, err := h.getSettings(ctx, &GetSettingsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in getSettings")
	}
	if _, err := h.getInboundToken(ctx, &GetInboundTokenInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in getInboundToken")
	}
	if _, err := h.getInstanceSettings(ctx, &GetInstanceSettingsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in getInstanceSettings")
	}
	if _, err := h.updateSettings(ctx, &UpdateSettingsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in updateSettings")
	}
	if _, err := h.updateInstanceSettings(ctx, &UpdateInstanceSettingsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in updateInstanceSettings")
	}
	if _, err := h.downloadBackup(ctx, &DownloadBackupInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in downloadBackup")
	}
}
