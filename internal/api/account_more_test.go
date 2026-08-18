package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

func TestAccountMore(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	// Create solitary user (not in any org) with related tables
	solitaryUser := models.User{Email: "solitary@example.com"}
	db.Create(&solitaryUser)
	db.Create(&models.UserIdentity{UserID: solitaryUser.ID, Provider: "github", Issuer: "https://github.com", Subject: "123"})
	db.Create(&models.UserSetting{UserID: solitaryUser.ID, Key: "theme", Value: "dark"})
	db.Create(&models.Token{UserID: solitaryUser.ID, Name: "User Tok", Hash: "hash"})
	solitaryCookies := sessionCookies(t, solitaryUser.ID, 0)

	// Create member user (belongs to org 1)
	orgUser := models.User{Email: "orguser@example.com"}
	db.Create(&orgUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: orgUser.ID, Role: "member"})
	orgUserCookies := sessionCookies(t, orgUser.ID, 1)

	// 1. Delete account unauth -> 401
	rec := do(srv, "DELETE", "/api/account/user", nil, `{"confirm":"DELETE MY ACCOUNT"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth delete account: got %d, want 401", rec.Code)
	}

	// 2. Delete account wrong confirmation string -> 400
	rec = do(srv, "DELETE", "/api/account/user", solitaryCookies, `{"confirm":"WRONG"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirm delete account: got %d, want 400", rec.Code)
	}

	// 3. Delete account when user belongs to an org -> 409 Conflict
	rec = do(srv, "DELETE", "/api/account/user", orgUserCookies, `{"confirm":"DELETE MY ACCOUNT"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete account in org: got %d, want 409", rec.Code)
	}

	// 4. Delete account for solitary user -> 200 OK (purges User, UserIdentity, UserSetting, Token, Sessions)
	rec = do(srv, "DELETE", "/api/account/user", solitaryCookies, `{"confirm":"DELETE MY ACCOUNT"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete solitary user: got %d (%s)", rec.Code, rec.Body.String())
	}
	var count int64
	db.Model(&models.User{}).Where("id = ?", solitaryUser.ID).Count(&count)
	if count != 0 {
		t.Errorf("solitary user still exists in DB after deletion")
	}

	// 5. Purge account invalid confirm -> 400
	rec = do(srv, "DELETE", "/api/account/data", adminCookies, `{"confirm":"WRONG"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("purge account wrong confirm: got %d, want 400", rec.Code)
	}

	// 6. Purge account non-owner -> 403
	rec = do(srv, "DELETE", "/api/account/data", orgUserCookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member purge account: got %d, want 403", rec.Code)
	}

	// 7. Successful Purge Account by Owner
	// Create dedicated org with all data types populated
	purgeOrg := models.Org{Name: "Purge Me Org", Slug: "purgemeorg"}
	db.Create(&purgeOrg)
	purgeOwner := models.User{Email: "purgeowner@example.com"}
	db.Create(&purgeOwner)
	db.Create(&models.OrgMember{OrgID: purgeOrg.ID, UserID: purgeOwner.ID, Role: "owner"})
	db.Create(&models.Token{OrgID: purgeOrg.ID, UserID: purgeOwner.ID, Name: "Org Token", Hash: "orghash"})
	db.Create(&models.NotificationChannel{OrgID: purgeOrg.ID, Name: "Ch", Type: "slack", Config: "{}"})
	db.Create(&models.WorkspaceSetting{OrgID: purgeOrg.ID, Key: "k", Value: "v"})
	db.Create(&models.PluginSetting{OrgID: purgeOrg.ID, Plugin: "links", Enabled: true})
	db.Create(&models.AuditLog{OrgID: purgeOrg.ID, Action: "purge.test"})
	db.Create(&models.Webhook{OrgID: purgeOrg.ID, Name: "wh", URL: "https://example.com/wh"})
	db.Create(&models.AbuseReport{OrgID: purgeOrg.ID, Slug: "abuse-slug"})
	db.Create(&models.Setting{Key: fmt.Sprintf("org_%d.stripe_key", purgeOrg.ID), Value: "sk_test_123"})

	purgeCookies := sessionCookies(t, purgeOwner.ID, purgeOrg.ID)
	rec = do(srv, "DELETE", "/api/account/data", purgeCookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge account successful: got %d (%s)", rec.Code, rec.Body.String())
	}
	db.Model(&models.Org{}).Where("id = ?", purgeOrg.ID).Count(&count)
	if count != 0 {
		t.Errorf("purged org still exists in DB")
	}

	// 8. Export account unauth -> 401
	rec = do(srv, "GET", "/api/account/export", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth export: got %d, want 401", rec.Code)
	}

	// 9. Export account member (non-admin) -> 403
	rec = do(srv, "GET", "/api/account/export", orgUserCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member export: got %d, want 403", rec.Code)
	}

	// 10. Export account admin -> 200 attachment
	rec = do(srv, "GET", "/api/account/export", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin export: got %d (%s)", rec.Code, rec.Body.String())
	}
	if disp := rec.Header().Get("Content-Disposition"); disp == "" {
		t.Errorf("missing Content-Disposition header in export")
	}

	// 11. Nil Ctx calls
	h, _, _ := newTestHandlerRaw(t)
	ctx := context.Background()
	if _, err := h.exportAccount(ctx, &ExportAccountInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in exportAccount")
	}
	if _, err := h.purgeAccount(ctx, &PurgeAccountInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in purgeAccount")
	}
	if _, err := h.deleteAccount(ctx, &DeleteAccountInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in deleteAccount")
	}
}

type dummyAccountPlugin struct{}

func (dummyAccountPlugin) Name() string                      { return "dummy" }
func (dummyAccountPlugin) Models() []any                     { return nil }
func (dummyAccountPlugin) Init(*plugin.Context) error        { return nil }
func (dummyAccountPlugin) Mount(plugin.Mux, *plugin.Context) {}

func TestAccountExportAndPurgeServices(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	adminCookies := loginCookies(t, srv)

	reg := plugin.NewRegistry()
	reg.Provide(plugin.ExportServiceName("dummy"), plugin.ExportFunc(func(orgID uint) map[string]any {
		return map[string]any{"dummy_data": "123"}
	}))
	reg.Provide(plugin.PurgeServiceName("dummy"), plugin.PurgeFunc(func(orgID uint) error {
		return nil
	}))
	h.SetServiceLookup(reg.Lookup)
	h.SetPlugins([]plugin.Plugin{dummyAccountPlugin{}})

	// Export with plugin
	rec := do(srv, "GET", "/api/account/export", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export with plugin: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Purge with plugin
	rec = do(srv, "DELETE", "/api/account/data", adminCookies, `{"confirm":"DELETE MY DATA"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge with plugin: got %d (%s)", rec.Code, rec.Body.String())
	}
}
