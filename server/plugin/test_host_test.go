package plugin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestTestHostAndMocks(t *testing.T) {
	// 1. Nil receivers
	var nilHost *plugin.TestHost
	if nilHost.Session() == nil || nilHost.Settings() == nil || nilHost.Events() == nil {
		t.Fatal("expected non-nil noop fallback from nil TestHost")
	}
	if nilHost.Crypto() != nil || nilHost.TenantDB(1) != nil {
		t.Fatal("expected nil Crypto and TenantDB from nil TestHost")
	}

	var nilSession *plugin.TestSession
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if nilSession.UserID(req) != 0 || nilSession.OrgID(req) != 0 || nilSession.OrgRole(req) != "" {
		t.Fatal("expected zero values from nil TestSession")
	}
	if !nilSession.RequireRole(req, "admin") || !nilSession.RequirePerm(req, "p", "a") {
		t.Fatal("expected true default from nil TestSession")
	}
	if nilSession.IsInstanceAdmin(req) || nilSession.RevokeUserOrgSessions(1, 1) != 0 {
		t.Fatal("expected false/0 from nil TestSession")
	}

	var nilCrypto *plugin.TestCrypto
	if enc, err := nilCrypto.Encrypt([]byte("plain")); err != nil || enc != "plain" {
		t.Fatalf("unexpected nilCrypto.Encrypt: %s, %v", enc, err)
	}
	if dec, err := nilCrypto.Decrypt("plain"); err != nil || string(dec) != "plain" {
		t.Fatalf("unexpected nilCrypto.Decrypt: %s, %v", string(dec), err)
	}

	var nilSettings *plugin.TestSettings
	if nilSettings.GetWorkspaceSetting(1, "k") != "" || nilSettings.GetGlobalSetting("k") != "" {
		t.Fatal("expected empty string from nil TestSettings")
	}
	if err := nilSettings.SetWorkspaceSetting(1, "k", "v"); err != nil {
		t.Fatal("expected nil error from nil TestSettings")
	}
	if err := nilSettings.SetGlobalSetting("k", "v"); err != nil {
		t.Fatal("expected nil error from nil TestSettings")
	}

	var nilEvents *plugin.TestEvents
	nilEvents.PublishEvent(1, "e", nil)
	nilEvents.RegisterWebhookEvent(plugin.WebhookEventDef{})
	nilEvents.OnEmail(nil)

	// 2. Empty structs (default mock behaviors)
	emptyHost := &plugin.TestHost{}
	if emptyHost.Session() == nil || emptyHost.Settings() == nil || emptyHost.Events() == nil {
		t.Fatal("expected non-nil noop fallback from empty TestHost")
	}
	if emptyHost.Crypto() != nil || emptyHost.TenantDB(1) != nil {
		t.Fatal("expected nil Crypto and TenantDB from empty TestHost")
	}

	emptySession := &plugin.TestSession{}
	if emptySession.UserID(req) != 0 || emptySession.OrgID(req) != 0 || emptySession.OrgRole(req) != "" {
		t.Fatal("expected zero values from empty TestSession")
	}
	if !emptySession.RequireRole(req, "admin") || !emptySession.RequirePerm(req, "p", "a") {
		t.Fatal("expected true default from empty TestSession")
	}
	if emptySession.IsInstanceAdmin(req) || emptySession.RevokeUserOrgSessions(1, 1) != 0 {
		t.Fatal("expected false/0 from empty TestSession")
	}

	emptyCrypto := &plugin.TestCrypto{}
	if enc, err := emptyCrypto.Encrypt([]byte("test")); err != nil || enc != "test" {
		t.Fatalf("unexpected emptyCrypto.Encrypt: %s, %v", enc, err)
	}
	if dec, err := emptyCrypto.Decrypt("test"); err != nil || string(dec) != "test" {
		t.Fatalf("unexpected emptyCrypto.Decrypt: %s, %v", string(dec), err)
	}

	emptySettings := &plugin.TestSettings{}
	if emptySettings.GetWorkspaceSetting(1, "k") != "" || emptySettings.GetGlobalSetting("k") != "" {
		t.Fatal("expected empty string from empty TestSettings")
	}
	if err := emptySettings.SetWorkspaceSetting(1, "k", "v"); err != nil {
		t.Fatal("expected nil error from empty TestSettings")
	}
	if err := emptySettings.SetGlobalSetting("k", "v"); err != nil {
		t.Fatal("expected nil error from empty TestSettings")
	}

	emptyEvents := &plugin.TestEvents{}
	emptyEvents.PublishEvent(1, "ev", nil)
	emptyEvents.RegisterWebhookEvent(plugin.WebhookEventDef{})
	emptyEvents.OnEmail(nil)

	// 3. Configured mock callbacks
	calledEvent := false
	calledEmail := false
	calledWebhook := false
	th := &plugin.TestHost{
		SessionMock: &plugin.TestSession{
			UserIDFn:          func(r *http.Request) uint { return 100 },
			OrgIDFn:           func(r *http.Request) uint { return 200 },
			OrgRoleFn:         func(r *http.Request) string { return "owner" },
			RequireRoleFn:     func(r *http.Request, min string) bool { return min == "owner" },
			RequirePermFn:     func(r *http.Request, p, min string) bool { return true },
			IsInstanceAdminFn: func(r *http.Request) bool { return true },
			RevokeSessionsFn:  func(u, o uint) int { return 5 },
		},
		CryptoMock: &plugin.TestCrypto{
			EncryptFn: func(b []byte) (string, error) { return "enc:" + string(b), nil },
			DecryptFn: func(s string) ([]byte, error) { return []byte("dec:" + s), nil },
		},
		SettingsMock: &plugin.TestSettings{
			GetWorkspaceSettingFn: func(o uint, k string) string { return "ws-" + k },
			SetWorkspaceSettingFn: func(o uint, k, v string) error { return errors.New("cannot set") },
			GetGlobalSettingFn:    func(k string) string { return "g-" + k },
			SetGlobalSettingFn:    func(k, v string) error { return errors.New("cannot set global") },
		},
		EventsMock: &plugin.TestEvents{
			PublishEventFn: func(orgID uint, event string, data any) {
				calledEvent = true
			},
			RegisterWebhookEventFn: func(def plugin.WebhookEventDef) {
				calledWebhook = true
			},
			OnEmailFn: func(fn func(plugin.EmailEvent)) {
				calledEmail = true
			},
		},
		TenantDBMock: func(orgID uint) *plugin.TenantDB {
			return nil
		},
	}

	if th.Session().UserID(req) != 100 || th.Session().OrgID(req) != 200 || th.Session().OrgRole(req) != "owner" {
		t.Fatal("unexpected values from th.Session()")
	}
	if !th.Session().RequireRole(req, "owner") || th.Session().RequireRole(req, "superadmin") {
		t.Fatal("unexpected RequireRole result")
	}
	if !th.Session().RequirePerm(req, "p", "m") || !th.Session().IsInstanceAdmin(req) || th.Session().RevokeUserOrgSessions(1, 1) != 5 {
		t.Fatal("unexpected RequirePerm / IsInstanceAdmin / RevokeUserOrgSessions result")
	}

	if enc, _ := th.Crypto().Encrypt([]byte("foo")); enc != "enc:foo" {
		t.Fatalf("unexpected encrypt: %s", enc)
	}
	if dec, _ := th.Crypto().Decrypt("foo"); string(dec) != "dec:foo" {
		t.Fatalf("unexpected decrypt: %s", string(dec))
	}

	if th.Settings().GetWorkspaceSetting(1, "bar") != "ws-bar" || th.Settings().GetGlobalSetting("foo") != "g-foo" {
		t.Fatal("unexpected settings get result")
	}
	if th.Settings().SetWorkspaceSetting(1, "bar", "val") == nil || th.Settings().SetGlobalSetting("foo", "val") == nil {
		t.Fatal("expected settings set error")
	}

	th.Events().PublishEvent(1, "test", nil)
	th.Events().RegisterWebhookEvent(plugin.WebhookEventDef{Key: "test"})
	th.Events().OnEmail(func(e plugin.EmailEvent) {})
	if !calledEvent || !calledWebhook || !calledEmail {
		t.Fatal("expected event spine callbacks to have run")
	}

	if th.TenantDB(1) != nil {
		t.Fatal("expected nil TenantDB")
	}
}

func TestContextHelpersAndTenantDB(t *testing.T) {
	// UserID context helpers
	ctx := context.Background()
	if plugin.UserIDFromContext(ctx) != 0 {
		t.Fatal("expected 0 UserID for background ctx")
	}
	ctx = plugin.WithUserID(ctx, 88)
	if plugin.UserIDFromContext(ctx) != 88 {
		t.Fatalf("expected 88 UserID, got %d", plugin.UserIDFromContext(ctx))
	}

	// TenantDBFor
	var nilCtx *plugin.Context
	if nilCtx.TenantDBFor(1) != nil {
		t.Fatal("expected nil TenantDB from nil Context")
	}
	emptyCtx := &plugin.Context{}
	if emptyCtx.TenantDBFor(1) != nil {
		t.Fatal("expected nil TenantDB from empty Context")
	}
	ctxWithTenant := &plugin.Context{
		TenantDB: func(orgID uint) *plugin.TenantDB {
			return nil
		},
	}
	if ctxWithTenant.TenantDBFor(1) != nil {
		t.Fatal("expected nil TenantDB result")
	}

	// NewTenantDB nil check
	if _, err := plugin.NewTenantDB(nil, 1); err == nil {
		t.Fatal("expected error from NewTenantDB with nil DB")
	}

	// RegisterTenantView
	plugin.RegisterTenantView(nil, plugin.TenantView{Name: "v"})
	plugin.RegisterTenantView(&plugin.Context{}, plugin.TenantView{Name: "v"})
	registered := false
	plugin.RegisterTenantView(&plugin.Context{
		RegisterTenantView: func(view plugin.TenantView) {
			registered = true
		},
	}, plugin.TenantView{Name: "v"})
	if !registered {
		t.Fatal("expected RegisterTenantView callback to fire")
	}
}
