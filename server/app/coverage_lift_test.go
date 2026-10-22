package app

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/api"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/plugin"
)

type testDummyCorePlugin struct{}

func (testDummyCorePlugin) Name() string          { return "core_dummy" }
func (testDummyCorePlugin) Describe() plugin.Info { return plugin.Info{Title: "Core", Core: true} }
func (testDummyCorePlugin) Models() []any         { return nil }
func (testDummyCorePlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	if mux != nil {
		mux.HandleFunc("GET /api/core_dummy", func(w http.ResponseWriter, r *http.Request) {})
	}
}

type testBadModelPlugin struct{}

func (testBadModelPlugin) Name() string                      { return "bad_model" }
func (testBadModelPlugin) Describe() plugin.Info             { return plugin.Info{Title: "Bad"} }
func (testBadModelPlugin) Models() []any                     { return []any{123} }
func (testBadModelPlugin) Mount(plugin.Mux, *plugin.Context) {}

type testFailingValidatorPlugin struct {
	testDummyCorePlugin
}

func (testFailingValidatorPlugin) Name() string { return "fail_val" }
func (testFailingValidatorPlugin) Validate(ctx context.Context, host plugin.Host) error {
	return errors.New("startup check failed")
}

func TestCoverageLift_HostRuntime_TenantDB(t *testing.T) {
	a := bootApp(t)
	h := &hostRuntime{db: a.gdb}

	if h.TenantDB(0) != nil {
		t.Fatal("expected nil TenantDB for orgID 0")
	}
	tdb := h.TenantDB(1)
	if tdb == nil {
		t.Fatal("expected non-nil TenantDB for orgID 1")
	}
}

func TestCoverageLift_PluginContext_Branches(t *testing.T) {
	a := bootApp(t)
	var capturedPrefix string

	pctx := a.BuildPluginContext(PluginContextTestParams{
		HandleStatic: func(prefix string, fsys fs.FS) {
			capturedPrefix = prefix
		},
		HttpMode: true,
		Plugins:  []plugin.Plugin{testDummyCorePlugin{}},
	})

	ctx := context.Background()

	// 1. Notification helpers with nil router
	_ = pctx.Notify(ctx, "email", "pt", "txt")
	_ = pctx.Emit(ctx, plugin.NotificationPayload{})
	_ = pctx.EmitTo(ctx, plugin.NotificationRecipient{}, plugin.NotificationPayload{})
	pctx.RegisterNotificationChannel(nil)
	pctx.RegisterNotifier("typ", nil)

	// 2. LLM, Task queue, Geo
	pctx.SetLLMResolverForOrg(nil)
	if err := pctx.Enqueue(ctx, "t", nil); err == nil {
		t.Fatal("expected error from Enqueue when queue is nil")
	}
	c, reg, city := pctx.GeoLookup("1.1.1.1")
	if c != "" || reg != "" || city != "" {
		t.Fatalf("expected empty geo results when geo resolver is nil")
	}

	// 3. Static files, Endpoints, Cron, Spans
	pctx.HandleStatic("/static", nil)
	if capturedPrefix != "/static" {
		t.Fatalf("expected static prefix /static, got %q", capturedPrefix)
	}
	if err := pctx.RegisterEndpoint(nil); err == nil {
		t.Fatal("expected error from RegisterEndpoint when engine is nil")
	}
	if err := pctx.RegisterCron("n", "s", nil); err == nil {
		t.Fatal("expected error from RegisterCron when engine is nil")
	}
	_, sp := pctx.StartSpan(ctx, "test-tracer", "test-span")
	if sp != nil {
		sp.End()
	}

	// 4. PluginActive and FeatureActive logic
	if !pctx.PluginActive(1, testDummyCorePlugin{}) {
		t.Fatal("expected core plugin to be active")
	}
	if pctx.PluginActive(1, testDummyPlugin{name: "noncore"}) {
		t.Fatal("expected noncore plugin to be inactive with nil apiHandler")
	}
	if !pctx.FeatureActive(0, "core_dummy") {
		t.Fatal("expected core feature to be active for org 0")
	}
	if pctx.FeatureActive(0, "noncore") {
		t.Fatal("expected noncore feature to be inactive for org 0")
	}
	if pctx.FeatureActive(1, "noncore") {
		t.Fatal("expected noncore feature to be inactive for org 1")
	}
}

func TestCoverageLift_Preflight_TableCollisionAndRoutes(t *testing.T) {
	// 1. Invalid model type in table collisions
	err := preflightTableCollisions(nil, []plugin.Plugin{testBadModelPlugin{}})
	if err == nil {
		t.Fatal("expected error from preflightTableCollisions with int model")
	}

	// 2. Multiple route collisions triggers line 251
	routes := newRouteRegistry()
	routes.claim("p1", "GET /api/dup1", false)
	routes.claim("p2", "GET /api/dup1", false)
	routes.claim("p1", "GET /api/dup2", false)
	routes.claim("p2", "GET /api/dup2", false)
	err = routes.Err()
	if err == nil || !strings.Contains(err.Error(), "2 route collisions") {
		t.Fatalf("expected error containing '2 route collisions', got: %v", err)
	}
}

func TestCoverageLift_BuiltinCron_Handlers(t *testing.T) {
	a := bootApp(t)
	_ = a.gdb.AutoMigrate(&models.Session{}, &models.Token{}, &models.AuditLog{}, &models.Setting{})

	cronHandlers := make(map[string]func(context.Context) error)
	pctx := &plugin.Context{
		RegisterCron: func(name, spec string, h func(context.Context) error) error {
			cronHandlers[name] = h
			return nil
		},
	}

	apiHandler := api.New(a.cfg, a.gdb, a.cipher, a.auth, a.geo, queue.New(""))
	a.setupBuiltinCron(pctx, apiHandler)

	ctx := context.Background()

	// 1. Session cleanup
	if h, ok := cronHandlers["session_cleanup"]; ok {
		if err := h(ctx); err != nil {
			t.Fatalf("session_cleanup cron failed: %v", err)
		}
	} else {
		t.Fatal("session_cleanup handler not registered")
	}

	// 2. Log rotation
	if h, ok := cronHandlers["log_rotation"]; ok {
		if err := h(ctx); err != nil {
			t.Fatalf("log_rotation cron failed: %v", err)
		}
	} else {
		t.Fatal("log_rotation handler not registered")
	}
}

func TestCoverageLift_PluginGate_NilRequest(t *testing.T) {
	a := bootApp(t)
	gate := a.pluginGate(nil)
	allowed, scoped := gate(nil, "some-feature")
	if allowed || scoped {
		t.Fatal("expected false, false when request is nil")
	}
}

func TestCoverageLift_GatedAdapter_AbortHandlerPanic(t *testing.T) {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig("T", "1.0.0"))
	ga := &gatedAdapter{
		Adapter: humaAPI.Adapter(),
		plugin:  "test-plugin",
		enabled: func(r *http.Request, plugin string) (bool, bool) {
			return true, true
		},
	}

	op := &huma.Operation{Method: http.MethodGet, Path: "/test-abort"}
	ga.Handle(op, func(ctx huma.Context) {
		panic(http.ErrAbortHandler)
	})

	defer func() {
		r := recover()
		if r != http.ErrAbortHandler {
			t.Fatalf("expected http.ErrAbortHandler panic to propagate, got %v", r)
		}
	}()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test-abort", nil)
	mux.ServeHTTP(rec, req)
}

func TestCoverageLift_NotificationConfigDecryptor(t *testing.T) {
	a := bootApp(t)
	_ = a.gdb.AutoMigrate(&models.Setting{}, &models.NotificationChannel{}, &models.Webhook{}, &models.User{})
	if err := a.cipher.EnableEnvelope(settingsStore{a.gdb}); err != nil {
		t.Fatalf("EnableEnvelope failed: %v", err)
	}

	// Decrypt invalid ciphertext -> fails
	_, err := notification.ConfigPlaintext("invalid-encrypted-blob")
	if err == nil {
		t.Fatal("expected error decrypting invalid ciphertext")
	}

	// Decrypt valid ciphertext -> succeeds
	enc, err := a.cipher.Encrypt([]byte("plaintext-cfg"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	pt, err := notification.ConfigPlaintext(enc)
	if err != nil || pt != "plaintext-cfg" {
		t.Fatalf("expected plaintext-cfg, got %q, err=%v", pt, err)
	}
}

func TestCoverageLift_RunMCP_CorePluginAndValidatorFailure(t *testing.T) {
	// 1. Core plugin mounting in RunMCP
	a := bootApp(t)
	a.Use(testDummyCorePlugin{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = a.RunMCP(ctx)

	// 2. Startup validation failure in RunMCP
	aVal := bootApp(t)
	aVal.Use(testFailingValidatorPlugin{})
	err := aVal.RunMCP(context.Background())
	if err == nil || !strings.Contains(err.Error(), "startup check failed") {
		t.Fatalf("expected startup check failed error, got: %v", err)
	}
}
