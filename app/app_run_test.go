package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/llmprovider"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/builtin"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/plugins/help"
	"github.com/octarq-org/octarq/plugins/links"
	"gorm.io/gorm"
)

// bootApp builds an App against a scratch SQLite database via the environment,
// exactly like the composition roots do.
func bootApp(t *testing.T) *App {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(tempDir, "boot.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "app-boot-secret-key-32-bytes-long!!!")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "app-boot-admin-pass")
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// providingPlugin is a stub plugin whose Mount Provides a fixed service name;
// two of them providing the same name must make the shared registry refuse boot.
type providingPlugin struct {
	name     string
	provides string
}

func (p providingPlugin) Name() string                            { return p.name }
func (p providingPlugin) Describe() plugin.Info                   { return plugin.Info{Title: p.name} }
func (p providingPlugin) Models() []any                           { return nil }
func (p providingPlugin) Mount(m plugin.Mux, ctx *plugin.Context) { ctx.Provide(p.provides, p.name) }

// requiringPlugin is a stub plugin whose Describe declares a Requires entry, so
// preflightDependencies can refuse a composition missing it.
type requiringPlugin struct {
	name     string
	requires []string
}

func (p requiringPlugin) Name() string { return p.name }
func (p requiringPlugin) Describe() plugin.Info {
	return plugin.Info{Title: p.name, Requires: p.requires}
}
func (p requiringPlugin) Models() []any                     { return nil }
func (p requiringPlugin) Mount(plugin.Mux, *plugin.Context) {}

// ctxProbePlugin exercises every plugin.Context closure during Mount so the
// wiring closures inside Run/RunMCP actually execute in tests rather than
// remaining dead code. It provides no services and asserts nothing itself; the
// calls are the point. Each call is gated on non-nil because the stdio MCP
// server re-mounts plugins with a minimal Context (DB/OrgID/Provide/Lookup only).
type ctxProbePlugin struct{ name string }

func (p ctxProbePlugin) Name() string          { return p.name }
func (p ctxProbePlugin) Describe() plugin.Info { return plugin.Info{Title: p.name} }
func (p ctxProbePlugin) Models() []any         { return nil }
func (p ctxProbePlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	if ctx.Guard != nil {
		ctx.Guard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)
	}
	if ctx.Notify != nil {
		ctx.Notify(context.Background(), "probe-chan", "{}", "hi")
	}
	if ctx.RegisterNotifier != nil {
		ctx.RegisterNotifier("probe-chan", func(context.Context, string, string) error { return nil })
	}
	if ctx.RegisterAuthMethod != nil {
		ctx.RegisterAuthMethod(plugin.AuthMethod{ID: "probe"})
	}
	if ctx.RevokeUserOrgSessions != nil {
		ctx.RevokeUserOrgSessions(1, 1)
	}
	if ctx.UserID != nil {
		_ = ctx.UserID(r)
	}
	if ctx.OrgID != nil {
		_ = ctx.OrgID(r)
	}
	if ctx.OrgRole != nil {
		_ = ctx.OrgRole(r)
	}
	if ctx.RequireRole != nil {
		ctx.RequireRole(r, "member")
	}
	if ctx.RequirePerm != nil {
		ctx.RequirePerm(r, "probe.perm", "member")
	}
	if ctx.IsInstanceAdmin != nil {
		ctx.IsInstanceAdmin(r)
	}
	if ctx.Audit != nil {
		ctx.Audit(r, "probe.action", "user", 1, nil)
	}
	if ctx.Encrypt != nil {
		if enc, err := ctx.Encrypt([]byte("secret")); err == nil {
			_, _ = ctx.Decrypt(enc)
		}
	}
	if ctx.SendMail != nil {
		_ = ctx.SendMail(1, "to@example.com", "sub", "<p>hi</p>", "hi")
	}
	if ctx.SetLLMResolver != nil {
		ctx.SetLLMResolver(func() (llmprovider.Provider, error) { return nil, nil })
	}
	if ctx.SetLLMResolverForOrg != nil {
		ctx.SetLLMResolverForOrg(func(uint) (llmprovider.Provider, error) { return nil, nil })
	}
	if ctx.GetWorkspaceSetting != nil {
		_ = ctx.GetWorkspaceSetting(1, "k")
	}
	if ctx.GetGlobalSetting != nil {
		_ = ctx.GetGlobalSetting("k")
	}
	if ctx.SetWorkspaceSetting != nil {
		_ = ctx.SetWorkspaceSetting(1, "k", "v")
	}
	if ctx.SetGlobalSetting != nil {
		_ = ctx.SetGlobalSetting("k", "v")
	}
	if ctx.RegisterTask != nil {
		ctx.RegisterTask("probe-task", func(context.Context, []byte) error { return nil })
	}
	if ctx.Enqueue != nil {
		_ = ctx.Enqueue(context.Background(), "probe-task", []byte(`{}`))
	}
	if ctx.CacheGet != nil {
		ctx.CacheGet(context.Background(), "k", new(string))
	}
	if ctx.CacheSet != nil {
		_ = ctx.CacheSet(context.Background(), "k", "v", time.Second)
	}
	if ctx.DeleteCache != nil {
		_ = ctx.DeleteCache(context.Background(), "k")
	}
	if ctx.GeoLookup != nil {
		ctx.GeoLookup("192.0.2.1")
	}
	if ctx.ParseUA != nil {
		_, _, _ = ctx.ParseUA("Mozilla/5.0")
	}
	if ctx.PublishEvent != nil {
		ctx.PublishEvent(1, "probe.event", map[string]any{"probe": true})
	}
	if ctx.RecordUsage != nil {
		ctx.RecordUsage(1, "clicks", 1)
	}
	if ctx.RegisterWebhookEvent != nil {
		ctx.RegisterWebhookEvent(plugin.WebhookEventDef{Key: "probe.event", Group: "g", Title: "t"})
	}
	if ctx.OnEmail != nil {
		ctx.OnEmail(func(plugin.EmailEvent) {})
	}
	if ctx.PluginActive != nil {
		ctx.PluginActive(1, p)
	}
	if ctx.FeatureActive != nil {
		ctx.FeatureActive(1, "anything")
	}
	if ctx.ActivePlugins != nil {
		_ = ctx.ActivePlugins()
	}
	if ctx.HandleRoot != nil {
		ctx.HandleRoot(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	}
	if ctx.HandleStatic != nil {
		ctx.HandleStatic("/probe", fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<!doctype html><html></html>")},
		})
	}
	if ctx.DNS != nil {
		_, _ = ctx.DNS.List(context.Background(), 1, 1)
		_, _ = ctx.DNS.Set(context.Background(), 1, 1, plugin.DNSRecord{})
		_ = ctx.DNS.Delete(context.Background(), 1, 1, "r1")
	}
	if ctx.LoginByEmail != nil {
		_, _ = ctx.LoginByEmail(w, r, "probe@"+p.name+".example.com")
	}
	if ctx.LoginByIdentity != nil {
		_, _ = ctx.LoginByIdentity(w, r, plugin.ExternalIdentity{
			Provider: "oidc", Issuer: "https://idp.probe.example", Subject: p.name,
			Email: "probe@" + p.name + ".example.com",
		})
	}
	if ctx.BindIdentity != nil {
		_ = ctx.BindIdentity(r, plugin.ExternalIdentity{Provider: "oidc", Issuer: "https://idp.probe.example", Subject: p.name})
	}
	if ctx.Provide != nil {
		ctx.Provide("probe.svc."+p.name, "value")
		_, _ = ctx.Lookup("probe.svc." + p.name)
	}
}

// servicePlugin Provides a single well-known service when mounted.
type servicePlugin struct {
	name string
	svc  string
	val  any
}

func (p servicePlugin) Name() string          { return p.name }
func (p servicePlugin) Describe() plugin.Info { return plugin.Info{Title: p.name} }
func (p servicePlugin) Models() []any         { return nil }
func (p servicePlugin) Mount(_ plugin.Mux, ctx *plugin.Context) {
	ctx.Provide(p.svc, p.val)
}

// appBootComposition is the plugin set the Run/RunMCP boot tests mount: real
// core plugins plus the probe and the service providers that unlock the code
// paths which only fire when a service exists. The ordering lets probe-a mount
// before the mail dispatcher exists (its OnEmail lands on the deferred list)
// while probe-b mounts after (its OnEmail takes the immediate branch), and the
// usage meter before either probe so RecordUsage finds it. help.New() is NOT
// here: the stdio MCP server re-mounts plugins with a Huma-less Context and
// help's Mount assumes one.
func appBootComposition() []plugin.Plugin {
	return []plugin.Plugin{
		dns.New(),
		links.New(),
		servicePlugin{name: "usage-meter", svc: plugin.ServiceCloudUsage, val: plugin.UsageMeter(func(uint, string, int64) {})},
		ctxProbePlugin{name: "probe-a"},
		servicePlugin{name: "mail-ready", svc: plugin.ServiceMailReady, val: plugin.MailReady(func() bool { return true })},
		servicePlugin{name: "mail-send", svc: plugin.ServiceMailSend, val: plugin.MailSender(func(uint, string, string, string, string) error { return nil })},
		servicePlugin{name: "dispatcher", svc: plugin.ServiceMailDispatcher, val: plugin.EmailDispatcher(func(func(plugin.EmailEvent)) {})},
		ctxProbePlugin{name: "probe-b"},
	}
}

// TestRunMCPWiring boots the MCP path (preflight, migrate, plugin mount) with a
// minimal composition and a cancelled context, so the stdio server returns
// immediately instead of blocking on stdin. It pins that plugins get a wired
// service registry and a migrated schema before the MCP server starts.
func TestRunMCPWiring(t *testing.T) {
	a := bootApp(t)
	for _, p := range appBootComposition() {
		a.Use(p)
	}
	a.Use(testDummyPlugin{name: "stub"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.RunMCP(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunMCP = %v, want context.Canceled", err)
	}
	if a.services == nil {
		t.Error("RunMCP did not create the service registry")
	}
	var tables int
	if err := a.gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tables).Error; err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables == 0 {
		t.Error("RunMCP ran no migrations")
	}
}

// TestRunBootsAndShutsDown boots the full HTTP composition root (core plugins
// plus one non-core stub) with a cancelled context. Run must migrate, serve
// briefly, and return nil on shutdown.
func TestRunBootsAndShutsDown(t *testing.T) {
	a := bootApp(t)
	for _, p := range appBootComposition() {
		a.Use(p)
	}
	a.Use(help.New())
	a.Use(testDummyPlugin{name: "stub"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	var tables int
	if err := a.gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tables).Error; err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables == 0 {
		t.Error("Run migrated no tables")
	}
}

// TestRunPreflightFailures drives each startup refusal Run surfaces before any
// request is served: duplicate plugin names, a missing Required plugin, two
// model types claiming one table, and two plugins Providing one service name.
func TestRunPreflightFailures(t *testing.T) {
	cases := []struct {
		name    string
		add     func(*App)
		wantSub string
	}{
		{
			name:    "duplicate plugin names",
			add:     func(a *App) { a.Use(testDummyPlugin{name: "dup"}); a.Use(testDummyPlugin{name: "dup"}) },
			wantSub: "two mounted plugins are both named",
		},
		{
			name:    "missing required plugin",
			add:     func(a *App) { a.Use(requiringPlugin{name: "needy", requires: []string{"missing"}}) },
			wantSub: "requires plugin",
		},
		{
			name: "table collision", add: func(a *App) {
				a.Use(testDummyPlugin{name: "pA", models: []any{customModelA{}}})
				a.Use(testDummyPlugin{name: "pB", models: []any{customModelB{}}})
			},
			wantSub: "declared by two different model types",
		},
		{
			name: "service name collision", add: func(a *App) {
				a.Use(providingPlugin{name: "one", provides: "svc.dup"})
				a.Use(providingPlugin{name: "two", provides: "svc.dup"})
			},
			wantSub: "provided twice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := bootApp(t)
			tc.add(a)
			if err := a.Run(context.Background()); err == nil {
				t.Fatal("Run succeeded, want preflight failure")
			} else if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("Run error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}

// TestRunMCPPreflightFailures drives the same startup refusals through the MCP
// path, which performs its own preflight before building the MCP server.
func TestRunMCPPreflightFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name    string
		add     func(*App)
		wantSub string
	}{
		{
			name:    "duplicate plugin names",
			add:     func(a *App) { a.Use(testDummyPlugin{name: "dup"}); a.Use(testDummyPlugin{name: "dup"}) },
			wantSub: "two mounted plugins are both named",
		},
		{
			name:    "missing required plugin",
			add:     func(a *App) { a.Use(requiringPlugin{name: "needy", requires: []string{"missing"}}) },
			wantSub: "requires plugin",
		},
		{
			name: "table collision", add: func(a *App) {
				a.Use(testDummyPlugin{name: "pA", models: []any{customModelA{}}})
				a.Use(testDummyPlugin{name: "pB", models: []any{customModelB{}}})
			},
			wantSub: "declared by two different model types",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := bootApp(t)
			tc.add(a)
			if err := a.RunMCP(ctx); err == nil {
				t.Fatal("RunMCP succeeded, want preflight failure")
			} else if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("RunMCP error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}

// TestNewErrorPaths covers app.New's configuration and database failure
// branches, plus the degraded-geo fallback that keeps New from erroring when
// the GeoIP database is missing.
func TestNewErrorPaths(t *testing.T) {
	t.Run("bad driver", func(t *testing.T) {
		t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
		if _, err := New(); err == nil {
			t.Fatal("New succeeded with an invalid DB driver")
		}
	})

	t.Run("unopenable database", func(t *testing.T) {
		t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
		t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "missing", "nest", "db.sqlite"))
		t.Setenv("OCTARQ_SECRET_KEY", "new-err-secret-key-32-bytes-long!!!")
		t.Setenv("OCTARQ_ADMIN_PASSWORD", "new-err-admin-pass")
		if _, err := New(); err == nil {
			t.Fatal("New succeeded with an unopenable database path")
		}
	})

	t.Run("missing geoip degrades", func(t *testing.T) {
		t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
		t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "geo.db"))
		t.Setenv("OCTARQ_SECRET_KEY", "new-err-geo-secret-key-32-bytes!!!")
		t.Setenv("OCTARQ_ADMIN_PASSWORD", "new-err-geo-admin-pass")
		t.Setenv("OCTARQ_GEOIP_DB", filepath.Join(t.TempDir(), "no-such-file.mmdb"))
		a, err := New()
		if err != nil {
			t.Fatalf("New with a broken geoip path = %v, want fallback", err)
		}
		if a.geo == nil {
			t.Error("geo resolver is nil after the fallback")
		}
	})
}

// TestRunSecretKeyFloor covers the post-mount refusal for a registered domain
// with a short secret key (enforceSecretKeyFloor), reached only after the
// domain is discoverable in the migrated schema.
func TestRunSecretKeyFloor(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "floor.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "short")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "floor-admin-pass")

	seed, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if err := seed.AutoMigrate(&dns.Domain{}); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	seed.Create(&dns.Domain{Name: "example.com", ForLink: true})
	sqlDB, _ := seed.DB()
	sqlDB.Close()

	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.Use(dns.New())
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded, want secret-key floor refusal")
	} else if !strings.Contains(err.Error(), "OCTARQ_SECRET_KEY must be at least") {
		t.Fatalf("Run error %q does not name the secret-key floor", err)
	}
}

// pluginEnabledStub stands in for the API handler's PluginEnabled in pluginGate
// tests.
type pluginEnabledStub struct {
	enabled func(orgID uint, key string) bool
}

func (s pluginEnabledStub) PluginEnabled(orgID uint, key string) bool {
	return s.enabled(orgID, key)
}

// TestPluginGate pins the org-0 contract (not workspace-scoped, never gated) and
// the delegation to the API handler's PluginEnabled for a real workspace.
func TestPluginGate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cfg := &config.Config{SecretKey: "secret", AdminUser: "admin", AdminPassword: "pw"}
	a := &App{auth: auth.New(cfg, crypto.New("secret")).WithDB(db)}

	gate := a.pluginGate(pluginEnabledStub{enabled: func(orgID uint, key string) bool { return true }})

	// No workspace in session -> not scoped, and the route stays unblocked.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if allowed, scoped := gate(r, "x"); allowed || scoped {
		t.Errorf("org-0 gate = (%v, %v), want (false, false)", allowed, scoped)
	}

	// A workspace present is gated through the API handler.
	r = r.WithContext(plugin.WithOrgID(r.Context(), 7))
	if allowed, scoped := gate(r, "x"); !allowed || !scoped {
		t.Errorf("org-7 gate = (%v, %v), want (true, true)", allowed, scoped)
	}

	gateOff := a.pluginGate(pluginEnabledStub{enabled: func(orgID uint, key string) bool { return false }})
	r = r.WithContext(plugin.WithOrgID(r.Context(), 9))
	if allowed, scoped := gateOff(r, "x"); allowed || !scoped {
		t.Errorf("org-9 disabled gate = (%v, %v), want (false, true)", allowed, scoped)
	}
}

// TestRecoverWriterFlushAndWrite exercises the stream-forwarding half of
// recoverWriter: headers/writes mark the written flag, Flush forwards to a
// flusher and is a no-op without one.
func TestRecoverWriterFlushAndWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &recoverWriter{ResponseWriter: rec}

	if n, err := rw.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("Write = (%d, %v), want (3, nil)", n, err)
	}
	if !rw.wroteHeader {
		t.Error("Write did not mark wroteHeader")
	}
	if rec.Body.String() != "abc" {
		t.Errorf("Write did not forward the body: %q", rec.Body.String())
	}

	// WriteHeader forwards, but the recorder has already implicitly written 200
	// on the Write above — a fresh writer records it directly.
	rec2 := httptest.NewRecorder()
	rw2 := &recoverWriter{ResponseWriter: rec2}
	rw2.WriteHeader(http.StatusTeapot)
	if rec2.Code != http.StatusTeapot {
		t.Errorf("WriteHeader did not forward to the underlying writer: got %d", rec2.Code)
	}

	// Flush forwards when the underlying writer implements http.Flusher.
	rw.Flush()
	if !rec.Flushed {
		t.Error("Flush did not reach the underlying http.Flusher")
	}

	// Flush without an http.Flusher must be a safe no-op.
	plain := &plainResponseWriter{new(bytes.Buffer)}
	rw3 := &recoverWriter{ResponseWriter: plain}
	rw3.Flush()
}

type plainResponseWriter struct{ buf *bytes.Buffer }

func (p *plainResponseWriter) Header() http.Header         { return http.Header{} }
func (p *plainResponseWriter) Write(b []byte) (int, error) { return p.buf.Write(b) }
func (p *plainResponseWriter) WriteHeader(int)             {}

// TestGatedMuxHandle covers the Handle method (as opposed to HandleFunc), which
// registers a gated handler under an exact pattern.
func TestGatedMuxHandle(t *testing.T) {
	real := http.NewServeMux()
	gm := &gatedMux{
		real:   real,
		plugin: "fake",
		enabled: func(_ *http.Request, _ string) (bool, bool) {
			return true, true
		},
	}
	gm.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	real.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("allowed Handle route = %d, want 200", rec.Code)
	}

	gmDis := &gatedMux{
		real:   real,
		plugin: "fake-dis",
		enabled: func(_ *http.Request, _ string) (bool, bool) {
			return false, true
		},
	}
	gmDis.Handle("/blocked", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	recDis := httptest.NewRecorder()
	real.ServeHTTP(recDis, httptest.NewRequest(http.MethodGet, "/blocked", nil))
	if recDis.Code != http.StatusNotFound {
		t.Fatalf("disabled Handle route = %d, want 404", recDis.Code)
	}
}

// TestGatedAdapterPanic verifies a panicking Huma handler registered through
// the gated adapter is recovered into a 500 JSON response instead of crashing
// the process.
func TestGatedAdapterPanic(t *testing.T) {
	realMux := http.NewServeMux()
	realAPI := humago.New(realMux, huma.DefaultConfig("Test API", "1.0.0"))
	gAPI := &gatedAPI{
		API: realAPI,
		gAdapter: &gatedAdapter{
			Adapter: realAPI.Adapter(),
			plugin:  "crash",
			enabled: func(_ *http.Request, _ string) (bool, bool) { return true, true },
		},
	}
	type In struct{}
	type Out struct{ Body struct{} }
	huma.Register(gAPI, huma.Operation{Method: "GET", Path: "/api/boom-huma"}, func(ctx context.Context, in *In) (*Out, error) {
		panic("huma boom")
	})

	rec := httptest.NewRecorder()
	realMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/boom-huma", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panicking huma route = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q, want application/json", ct)
	}
}

// appAuthDB builds an App with an in-memory auth manager and migrated schema
// for the login delegate tests.
func appAuthDB(t *testing.T) (*App, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:appauth-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Drop the shared in-memory DB on cleanup so -count=2 re-runs stay isolated.
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{SecretKey: "secret", AdminUser: "admin", AdminPassword: "pw"}
	c := crypto.New("secret")
	m := auth.New(cfg, c).WithDB(db)
	return &App{cfg: cfg, cipher: c, auth: m}, db
}

func seedIdentityUser(t *testing.T, db *gorm.DB) (models.User, models.Org) {
	t.Helper()
	slug, err := models.AllocateOrgSlug(db)
	if err != nil {
		t.Fatalf("slug: %v", err)
	}
	u := models.User{Email: "sso@acme.com", EmailVerified: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("user: %v", err)
	}
	o := models.Org{Name: "Acme", Slug: slug, InboundToken: "tok"}
	if err := db.Create(&o).Error; err != nil {
		t.Fatalf("org: %v", err)
	}
	if err := db.Create(&models.OrgMember{OrgID: o.ID, UserID: u.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("member: %v", err)
	}
	return u, o
}

// TestLoginDelegates maps the app-level login methods onto the auth manager:
// registration-disabled and account-link refusals must translate to the
// plugin-package sentinels, and a bound identity completes a session.
func TestLoginDelegates(t *testing.T) {
	t.Run("registration disabled maps to plugin sentinel", func(t *testing.T) {
		a, db := appAuthDB(t)
		if err := db.Create(&models.Setting{Key: "allow_registration", Value: "false"}).Error; err != nil {
			t.Fatalf("setting: %v", err)
		}
		id := plugin.ExternalIdentity{
			Provider: "oidc", Issuer: "https://idp.acme.com", Subject: "s1",
			Email: "new@acme.com", MayCreateUser: true,
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		_, err := a.loginByIdentity(rec, req, id)
		if !errors.Is(err, plugin.ErrLoginRegistrationDisabled) {
			t.Fatalf("loginByIdentity disabled = %v, want plugin.ErrLoginRegistrationDisabled", err)
		}
		_, err = a.loginByEmail(rec, req, "stranger@acme.com")
		if !errors.Is(err, plugin.ErrLoginRegistrationDisabled) {
			t.Fatalf("loginByEmail disabled = %v, want plugin.ErrLoginRegistrationDisabled", err)
		}
	})

	t.Run("account link required maps to plugin sentinel", func(t *testing.T) {
		a, db := appAuthDB(t)
		seedIdentityUser(t, db)
		id := plugin.ExternalIdentity{
			Provider: "oidc", Issuer: "https://evil.example", Subject: "s2",
			Email: "sso@acme.com", MayCreateUser: true,
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		_, err := a.loginByIdentity(rec, req, id)
		if !errors.Is(err, plugin.ErrAccountLinkRequired) {
			t.Fatalf("loginByIdentity = %v, want plugin.ErrAccountLinkRequired", err)
		}
	})

	t.Run("bound identity succeeds and issues a session", func(t *testing.T) {
		a, db := appAuthDB(t)
		u, o := seedIdentityUser(t, db)
		id := plugin.ExternalIdentity{
			Provider: "oidc", Issuer: "https://idp.acme.com", Subject: "s3",
			Email: "sso@acme.com", OrgID: o.ID,
		}
		if err := db.Create(&models.UserIdentity{UserID: u.ID, Provider: "oidc", Issuer: "https://idp.acme.com", Subject: "s3", Email: "sso@acme.com"}).Error; err != nil {
			t.Fatalf("bind: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		uid, err := a.loginByIdentity(rec, req, id)
		if err != nil {
			t.Fatalf("loginByIdentity: %v", err)
		}
		if uid != u.ID {
			t.Errorf("uid = %d, want %d", uid, u.ID)
		}
		if len(rec.Result().Cookies()) == 0 {
			t.Error("login issued no session cookie")
		}
	})
}

// TestAppNotify routes through the notify registry: an unknown channel type is
// refused, and a registered provider receives the message text.
func TestAppNotify(t *testing.T) {
	a := &App{}
	ctx := context.Background()

	// notify.Send decrypts stored channel config before dispatch; a bare App{}
	// has no decryptor registered, so install a passthrough for this test.
	notify.SetConfigDecryptor(func(stored string) (string, bool) { return stored, true })

	if err := a.Notify(ctx, "apptest-nosuch", "{}", "hi"); err == nil {
		t.Fatal("Notify accepted an unregistered channel type")
	}

	var gotText string
	notify.Register("apptest-chan", func(_ context.Context, _, text string) error {
		gotText = text
		return nil
	})
	if err := a.Notify(ctx, "apptest-chan", "{}", "hello world"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotText != "hello world" {
		t.Errorf("provider received %q, want hello world", gotText)
	}
}

// fakeDNSManager satisfies plugin.DNSManager for the lazy-resolver happy path.
type fakeDNSManager struct{}

func (fakeDNSManager) List(context.Context, uint, uint) ([]plugin.DNSRecord, error) {
	return nil, nil
}
func (fakeDNSManager) Set(context.Context, uint, uint, plugin.DNSRecord) (plugin.DNSRecord, error) {
	return plugin.DNSRecord{}, nil
}
func (fakeDNSManager) Delete(context.Context, uint, uint, string) error { return nil }

// TestLazyDNSResolution covers resolve()'s type-assertion guard and the three
// delegated operations through a resolvable manager.
func TestLazyDNSResolution(t *testing.T) {
	ctx := context.Background()
	missing := &lazyDNSManager{lookup: func(string) (any, bool) { return nil, false }}
	if _, err := missing.List(ctx, 1, 1); err == nil {
		t.Error("List on unmounted manager = nil")
	}

	wrongType := &lazyDNSManager{lookup: func(string) (any, bool) { return "not-a-dns-manager", true }}
	if _, err := wrongType.List(ctx, 1, 1); err == nil {
		t.Error("List with a wrongly-typed service = nil")
	}

	ok := &lazyDNSManager{lookup: func(string) (any, bool) { return fakeDNSManager{}, true }}
	if _, err := ok.List(ctx, 1, 1); err != nil {
		t.Errorf("List: %v", err)
	}
	if _, err := ok.Set(ctx, 1, 1, plugin.DNSRecord{}); err != nil {
		t.Errorf("Set: %v", err)
	}
	if err := ok.Delete(ctx, 1, 1, "r1"); err != nil {
		t.Errorf("Delete: %v", err)
	}
}

// TestBuiltinDefaultMonitor ensures the default open-core composition used by
// main() still wires through the mount loop used by Run.
func TestBuiltinDefaultMonitor(t *testing.T) {
	a := bootApp(t)
	for _, p := range builtin.Default() {
		a.Use(p)
	}
	if len(a.Plugins()) != len(builtin.Default()) {
		t.Fatalf("Use did not accumulate %d plugins", len(builtin.Default()))
	}
}
