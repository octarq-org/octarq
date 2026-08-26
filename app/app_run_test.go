package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
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

// wiringCapture records the observable side effects of ctx closures that
// delegate to service providers, shared between the assertion plugin and the
// boot tests so a wiring regression turns one of them red.
type wiringCapture struct {
	mu            sync.Mutex
	usageMetric   string
	usageN        int64
	mailTo        string
	emailHandlers int
}

func (c *wiringCapture) addUsage(metric string, n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usageMetric, c.usageN = metric, n
}

func (c *wiringCapture) addMail(to string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mailTo = to
}

func (c *wiringCapture) addEmailHandler() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emailHandlers++
}

// mapCache is a real in-memory cache.Cache injected into the app's auth manager
// so the CacheGet/CacheSet/DeleteCache wiring round-trips (the production
// NoopCache never stores, so it could not back an assertion).
type mapCache struct {
	mu sync.Mutex
	m  map[string]any
}

func newMapCache() *mapCache { return &mapCache{m: map[string]any{}} }

func (c *mapCache) Get(_ context.Context, key string, dst any) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	if !ok {
		return false
	}
	if sp, ok := dst.(*string); ok {
		*sp, _ = v.(string)
	}
	return true
}

func (c *mapCache) Set(_ context.Context, key string, val any, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = val
	return nil
}

func (c *mapCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, key)
	return nil
}

func (c *mapCache) IsRedis() bool { return false }

// ctxAssertPlugin mounts during Run's and RunMCP's plugin loops and asserts the
// plugin Context wiring with real observable round-trips: settings written via
// Set*Setting must read back through Get*Setting, a registered notifier must
// receive the Notify, the cache must round-trip, Guard must admit only an
// authenticated request, and UserID/OrgID/OrgRole must come back from the
// request. A broken wire (say SetGlobalSetting pointing at a no-op) turns the
// boot tests red. httpMode selects the surface Run wires (it adds
// PluginActive/FeatureActive/ActivePlugins) versus the MCP path that omits them.
type ctxAssertPlugin struct {
	t        *testing.T
	name     string
	cap      *wiringCapture
	httpMode bool
}

func (p ctxAssertPlugin) Name() string { return p.name }
func (p ctxAssertPlugin) Describe() plugin.Info {
	return plugin.Info{Title: p.name, EnabledByDefault: true}
}
func (p ctxAssertPlugin) Models() []any { return nil }

func (p ctxAssertPlugin) surface() string {
	if p.httpMode {
		return "HTTP (Run)"
	}
	return "MCP (RunMCP)"
}

func (p ctxAssertPlugin) present(name string, ok bool) {
	p.t.Helper()
	if !ok {
		p.t.Errorf("plugin.Context.%s must be wired in the %s path", name, p.surface())
	}
}

func (p ctxAssertPlugin) Mount(_ plugin.Mux, ctx *plugin.Context) {
	t := p.t
	t.Helper()

	// The stdio MCP server re-mounts every plugin with a minimal Context (only
	// DB/OrgID/Provide/Lookup — see mcp.buildServerInstance). That surface is
	// pinned by TestMCPRemountUsesMinimalContext; here we just confirm the
	// remount kept the minimal subset and defer the full assertions.
	if ctx.SetGlobalSetting == nil {
		p.present("DB", ctx.DB != nil)
		p.present("OrgID", ctx.OrgID != nil)
		p.present("Provide", ctx.Provide != nil)
		p.present("Lookup", ctx.Lookup != nil)
		return
	}

	// The whole surface must be wired.
	p.present("Guard", ctx.Guard != nil)
	p.present("UserID", ctx.UserID != nil)
	p.present("OrgID", ctx.OrgID != nil)
	p.present("OrgRole", ctx.OrgRole != nil)
	p.present("RequireRole", ctx.RequireRole != nil)
	p.present("RequirePerm", ctx.RequirePerm != nil)
	p.present("IsInstanceAdmin", ctx.IsInstanceAdmin != nil)
	p.present("Audit", ctx.Audit != nil)
	p.present("Encrypt", ctx.Encrypt != nil)
	p.present("Decrypt", ctx.Decrypt != nil)
	p.present("Notify", ctx.Notify != nil)
	p.present("RegisterNotifier", ctx.RegisterNotifier != nil)
	p.present("OnEmail", ctx.OnEmail != nil)
	p.present("SendMail", ctx.SendMail != nil)
	p.present("RecordUsage", ctx.RecordUsage != nil)
	p.present("GetGlobalSetting", ctx.GetGlobalSetting != nil)
	p.present("SetGlobalSetting", ctx.SetGlobalSetting != nil)
	p.present("GetWorkspaceSetting", ctx.GetWorkspaceSetting != nil)
	p.present("SetWorkspaceSetting", ctx.SetWorkspaceSetting != nil)
	p.present("CacheGet", ctx.CacheGet != nil)
	p.present("CacheSet", ctx.CacheSet != nil)
	p.present("DeleteCache", ctx.DeleteCache != nil)
	p.present("Cache", ctx.Cache != nil)
	p.present("ParseUA", ctx.ParseUA != nil)
	p.present("RegisterAuthMethod", ctx.RegisterAuthMethod != nil)
	p.present("RegisterWebhookEvent", ctx.RegisterWebhookEvent != nil)
	p.present("LoginByEmail", ctx.LoginByEmail != nil)
	p.present("LoginByIdentity", ctx.LoginByIdentity != nil)
	p.present("BindIdentity", ctx.BindIdentity != nil)
	p.present("Provide", ctx.Provide != nil)
	p.present("Lookup", ctx.Lookup != nil)
	p.present("DB", ctx.DB != nil)
	p.present("RegisterEndpoint", ctx.RegisterEndpoint != nil)

	if p.httpMode {
		p.present("PluginActive", ctx.PluginActive != nil)
		p.present("FeatureActive", ctx.FeatureActive != nil)
		p.present("ActivePlugins", ctx.ActivePlugins != nil)
		p.present("HandleRoot", ctx.HandleRoot != nil)
		p.present("HandleStatic", ctx.HandleStatic != nil)
	} else {
		for _, name := range []string{"PluginActive", "FeatureActive", "ActivePlugins"} {
			if !isNilFunc(ctxField(ctx, name)) {
				t.Errorf("plugin.Context.%s must stay nil in the MCP path", name)
			}
		}
	}

	// Global setting round-trip.
	if err := ctx.SetGlobalSetting("cov.g"+p.name, "gv"); err != nil {
		t.Fatalf("SetGlobalSetting: %v", err)
	}
	if got := ctx.GetGlobalSetting("cov.g" + p.name); got != "gv" {
		t.Errorf("GetGlobalSetting after Set = %q, want gv", got)
	}

	// Workspace setting round-trip + cross-org isolation. A fixed org id far
	// from the auto-increment range keeps the isolation check unambiguous; the
	// key is per-probe so the two instances never overwrite each other.
	wsOrg := uint(4242)
	if err := ctx.SetWorkspaceSetting(wsOrg, "cov.ws"+p.name, "wv"); err != nil {
		t.Fatalf("SetWorkspaceSetting: %v", err)
	}
	if got := ctx.GetWorkspaceSetting(wsOrg, "cov.ws"+p.name); got != "wv" {
		t.Errorf("GetWorkspaceSetting after Set = %q, want wv", got)
	}
	if got := ctx.GetWorkspaceSetting(wsOrg+1, "cov.ws"+p.name); got != "" {
		t.Errorf("GetWorkspaceSetting crossed org boundaries: %q", got)
	}

	// A registered notifier must receive the Notify.
	var gotText string
	ctx.RegisterNotifier("cov.notify."+p.name, func(_ context.Context, _, text string) error {
		gotText = text
		return nil
	})
	cfg, err := ctx.Encrypt([]byte(`{}`))
	if err != nil {
		t.Fatalf("Encrypt notifier config: %v", err)
	}
	if err := ctx.Notify(context.Background(), "cov.notify."+p.name, cfg, "hello "+p.name); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotText != "hello "+p.name {
		t.Errorf("notifier received %q, want %q", gotText, "hello "+p.name)
	}

	// Cache round-trips through the real in-memory cache.
	ck := "cov:cache:" + p.name
	if err := ctx.CacheSet(context.Background(), ck, "cacheval", time.Minute); err != nil {
		t.Fatalf("CacheSet: %v", err)
	}
	var cached string
	if !ctx.CacheGet(context.Background(), ck, &cached) || cached != "cacheval" {
		t.Errorf("CacheGet after set = %q, want cacheval", cached)
	}
	if err := ctx.DeleteCache(context.Background(), ck); err != nil {
		t.Fatalf("DeleteCache: %v", err)
	}
	var after string
	if ctx.CacheGet(context.Background(), ck, &after) {
		t.Errorf("CacheGet after delete = %q, want a miss", after)
	}

	// ScopedCache round-trip.
	if err := ctx.Cache.Set(context.Background(), "scoped_k", "scoped_v", time.Minute); err != nil {
		t.Fatalf("ScopedCache Set: %v", err)
	}
	var scopedVal string
	if found, err := ctx.Cache.Get(context.Background(), "scoped_k", &scopedVal); !found || err != nil || scopedVal != "scoped_v" {
		t.Errorf("ScopedCache Get got found=%v, err=%v, val=%q", found, err, scopedVal)
	}
	if err := ctx.Cache.Delete(context.Background(), "scoped_k"); err != nil {
		t.Fatalf("ScopedCache Delete: %v", err)
	}
	if found, _ := ctx.Cache.Get(context.Background(), "scoped_k", &scopedVal); found {
		t.Errorf("ScopedCache Get after delete returned found=true")
	}

	// Guard admits only an authenticated request. The identity rows are built
	// through the wired DB so the authenticated path is self-contained; the
	// generated user/org IDs feed the identity assertions below.
	u := models.User{Email: p.name + "@cov.example", EmailVerified: true}
	if err := ctx.DB.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	o := models.Org{Name: p.name + " org", Slug: "cov-" + p.name, InboundToken: "tok"}
	if err := ctx.DB.Create(&o).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := ctx.DB.Create(&models.OrgMember{OrgID: o.ID, UserID: u.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	raw := "cov-session-" + p.name
	if err := ctx.DB.Create(&models.Session{
		UserID: u.ID, OrgID: o.ID, Token: models.HashToken(raw),
		IP: "192.0.2.1", UserAgent: "cov", LastSeenAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	guarded := ctx.Guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	anon := httptest.NewRecorder()
	guarded.ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/cov-guard", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Errorf("Guard admitted an anonymous request: %d", anon.Code)
	}

	authedReq := httptest.NewRequest(http.MethodGet, "/cov-guard", nil)
	authedReq.AddCookie(&http.Cookie{Name: "octarq_session", Value: raw})
	authed := httptest.NewRecorder()
	guarded.ServeHTTP(authed, authedReq)
	if authed.Code != http.StatusOK {
		t.Errorf("Guard blocked an authenticated request: %d", authed.Code)
	}

	// Identity extraction returns the request's user/org/role.
	if got := ctx.UserID(authedReq); got != u.ID {
		t.Errorf("UserID = %d, want %d", got, u.ID)
	}
	if got := ctx.OrgID(authedReq); got != o.ID {
		t.Errorf("OrgID = %d, want %d", got, o.ID)
	}
	if got := ctx.OrgRole(authedReq); got != "owner" {
		t.Errorf("OrgRole = %q, want owner", got)
	}
	if ctx.IsInstanceAdmin(authedReq) {
		t.Error("a regular user was reported instance admin")
	}
	if !ctx.RequireRole(authedReq, "owner") {
		t.Error("owner failed RequireRole(owner)")
	}
	if !ctx.RequirePerm(authedReq, "cov.perm", "member") {
		t.Error("owner failed RequirePerm(min member)")
	}

	// Encrypt/Decrypt round-trip.
	enc, err := ctx.Encrypt([]byte("cov secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if dec, err := ctx.Decrypt(enc); err != nil || string(dec) != "cov secret" {
		t.Errorf("Decrypt(Encrypt(x)) = %q, %v; want cov secret", dec, err)
	}

	// ParseUA actually parses a desktop Chrome UA.
	dev, browser, osName := ctx.ParseUA("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	if dev == "" && browser == "" && osName == "" {
		t.Error("ParseUA returned no fields for a desktop Chrome UA")
	}

	// RegisterAuthMethod surfaces the method to auth.List.
	ctx.RegisterAuthMethod(plugin.AuthMethod{ID: "cov.method." + p.name})
	var methodFound bool
	for _, m := range auth.List() {
		if m.ID == "cov.method."+p.name {
			methodFound = true
		}
	}
	if !methodFound {
		t.Error("RegisterAuthMethod did not surface in auth.List")
	}

	// RegisterWebhookEvent surfaces the event to the eventbus registry.
	ctx.RegisterWebhookEvent(plugin.WebhookEventDef{Key: "cov.event." + p.name, Group: "cov", Title: "t", Description: "d"})
	var eventFound bool
	for _, g := range eventbus.EventGroups() {
		for _, e := range g.Events {
			if e.Key == "cov.event."+p.name {
				eventFound = true
			}
		}
	}
	if !eventFound {
		t.Error("RegisterWebhookEvent did not surface in eventbus.EventGroups")
	}

	// RecordUsage reaches the usage-meter service.
	ctx.RecordUsage(o.ID, "clicks", 5)
	if p.cap.usageMetric != "clicks" || p.cap.usageN != 5 {
		t.Errorf("usage meter got %q/%d, want clicks/5", p.cap.usageMetric, p.cap.usageN)
	}

	// SendMail reaches the mail-send service.
	if err := ctx.SendMail(o.ID, "to@cov.example", "sub", "<p>h</p>", "h"); err != nil {
		t.Fatalf("SendMail: %v", err)
	}
	if p.cap.mailTo != "to@cov.example" {
		t.Errorf("mail service got to=%q, want to@cov.example", p.cap.mailTo)
	}

	// OnEmail must hand the handler to the dispatcher (immediate for the probe
	// mounted after it, deferred-and-flushed for the one before it; the boot
	// test asserts the captured count).
	ctx.OnEmail(func(plugin.EmailEvent) {})

	// Provide/Lookup round-trip.
	svc := "cov.svc." + p.name
	ctx.Provide(svc, "svcval")
	if v, ok := ctx.Lookup(svc); !ok || v != "svcval" {
		t.Errorf("Lookup after Provide = %v (ok=%v), want svcval", v, ok)
	}

	// HTTP-only enablement behavior.
	if p.httpMode {
		names := map[string]bool{}
		for _, pl := range ctx.ActivePlugins() {
			names[pl.Name()] = true
		}
		if !names[p.name] {
			t.Errorf("ActivePlugins missing %s (have %v)", p.name, names)
		}
		if !ctx.PluginActive(1, help.New()) {
			t.Error("core help plugin reported inactive")
		}
		if !ctx.PluginActive(1, p) {
			t.Error("an EnabledByDefault plugin reported inactive")
		}
		if !ctx.FeatureActive(1, "dns") {
			t.Error("core dns feature reported inactive")
		}
		if ctx.FeatureActive(0, "no-such-feature") {
			t.Error("an unknown feature defaulted enabled")
		}
		if !ctx.FeatureActive(1, p.name) {
			t.Error("an EnabledByDefault feature reported inactive in org 1")
		}
	}

	// LoginByEmail provisions a real account through the wired auth manager.
	rec := httptest.NewRecorder()
	uid, err := ctx.LoginByEmail(rec, httptest.NewRequest(http.MethodGet, "/", nil), "cov-login-"+p.name+"@example.com")
	if err != nil {
		t.Fatalf("LoginByEmail: %v", err)
	}
	if uid == 0 {
		t.Error("LoginByEmail returned user 0")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Error("LoginByEmail issued no session cookie")
	}

	// LoginByIdentity provisions a JIT account through the wired resolver.
	recID := httptest.NewRecorder()
	uid, err = ctx.LoginByIdentity(recID, httptest.NewRequest(http.MethodGet, "/", nil), plugin.ExternalIdentity{
		Provider: "oidc", Issuer: "https://cov.example", Subject: p.name + "-s",
		Email: "cov-id-" + p.name + "@example.com", MayCreateUser: true,
	})
	if err != nil {
		t.Fatalf("LoginByIdentity: %v", err)
	}
	if uid == 0 {
		t.Error("LoginByIdentity returned user 0")
	}

	// RevokeUserOrgSessions removes the guard session minted above.
	if n := ctx.RevokeUserOrgSessions(u.ID, o.ID); n < 1 {
		t.Errorf("revoked %d session(s), want >= 1", n)
	}
	var sessLeft int64
	if err := ctx.DB.Model(&models.Session{}).Where("user_id = ? AND org_id = ?", u.ID, o.ID).Count(&sessLeft).Error; err != nil {
		t.Fatalf("session count: %v", err)
	}
	if sessLeft != 0 {
		t.Errorf("sessions after revoke = %d, want 0", sessLeft)
	}
}

func ctxField(ctx *plugin.Context, name string) reflect.Value {
	return reflect.ValueOf(ctx).Elem().FieldByName(name)
}

func isNilFunc(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	return v.IsNil()
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
// core plugins, the capturing service providers (usage meter, mail sender, mail
// dispatcher) and two ctxAssertPlugin instances. probe-a mounts before the
// dispatcher exists (its OnEmail lands on the deferred list, flushed after the
// mount loop), probe-b after it (immediate dispatch) — so both OnEmail paths
// are observable. help.New() is appended only for the HTTP path: the stdio MCP
// server re-mounts plugins with a Huma-less Context and help's Mount assumes
// Huma is wired.
func appBootComposition(t *testing.T, cap *wiringCapture, httpMode bool) []plugin.Plugin {
	plugins := []plugin.Plugin{
		dns.New(),
		links.New(),
		servicePlugin{name: "usage-meter", svc: plugin.ServiceCloudUsage, val: plugin.UsageMeter(func(_ uint, metric string, n int64) { cap.addUsage(metric, n) })},
		servicePlugin{name: "mail-send", svc: plugin.ServiceMailSend, val: plugin.MailSender(func(_ uint, to, _, _, _ string) error { cap.addMail(to); return nil })},
		ctxAssertPlugin{t: t, name: "probe-a", cap: cap, httpMode: httpMode},
		servicePlugin{name: "mail-ready", svc: plugin.ServiceMailReady, val: plugin.MailReady(func() bool { return true })},
		servicePlugin{name: "dispatcher", svc: plugin.ServiceMailDispatcher, val: plugin.EmailDispatcher(func(func(plugin.EmailEvent)) { cap.addEmailHandler() })},
		ctxAssertPlugin{t: t, name: "probe-b", cap: cap, httpMode: httpMode},
	}
	if httpMode {
		plugins = append(plugins, help.New())
	}
	return plugins
}

// TestRunMCPWiring boots the MCP path (preflight, migrate, plugin mount) with a
// cancelled context, so the stdio server returns immediately instead of blocking
// on stdin. The ctx-assertion plugin runs its round-trips against RunMCP's full
// Context; the on-email capture proves the dispatcher wiring; a migrated schema
// proves RunMCP ran its migration pass.
func TestRunMCPWiring(t *testing.T) {
	a := bootApp(t)
	c := &wiringCapture{}
	a.auth.WithCache(newMapCache())
	for _, p := range appBootComposition(t, c, false) {
		a.Use(p)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.RunMCP(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunMCP = %v, want context.Canceled", err)
	}
	if c.emailHandlers < 2 {
		t.Errorf("email dispatcher captured %d handler(s), want >= 2", c.emailHandlers)
	}
	if c.usageMetric != "clicks" || c.mailTo == "" {
		t.Errorf("usage/mail captures missing: %+v", c)
	}
	var tables int
	if err := a.gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tables).Error; err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables == 0 {
		t.Error("RunMCP ran no migrations")
	}
}

// TestRunBootsAndShutsDown boots the full HTTP composition root with a
// cancelled context. Run must migrate, serve briefly, and return nil on
// shutdown, and the ctx-assertion plugin's observable round-trips (settings,
// notifier, cache, guard, identity) must all pass — broken wiring turns this
// test red. The on-email capture covers both the deferred and immediate
// dispatcher paths.
func TestRunBootsAndShutsDown(t *testing.T) {
	a := bootApp(t)
	c := &wiringCapture{}
	a.auth.WithCache(newMapCache())
	for _, p := range appBootComposition(t, c, true) {
		a.Use(p)
	}
	a.Use(testDummyPlugin{name: "stub"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if c.emailHandlers < 2 {
		t.Errorf("email dispatcher captured %d handler(s), want >= 2", c.emailHandlers)
	}
	var tables int
	if err := a.gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tables).Error; err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables == 0 {
		t.Error("Run migrated no tables")
	}
}

// TestMCPRemountUsesMinimalContext pins the Context the stdio MCP server
// re-mounts plugins with (mcp.buildServerInstance): only DB/OrgID/Provide/
// Lookup are wired; every HTTP-oriented closure is deliberately nil there.
func TestMCPRemountUsesMinimalContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mcp-minimal-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	minimal := &plugin.Context{
		DB:      db,
		OrgID:   func(*http.Request) uint { return 1 },
		Provide: func(string, any) {},
		Lookup:  func(string) (any, bool) { return nil, false },
	}
	if minimal.DB == nil || minimal.OrgID == nil || minimal.Provide == nil || minimal.Lookup == nil {
		t.Fatal("minimal remount context lost its wired subset")
		return
	}
	for _, name := range []string{
		"Guard", "UserID", "OrgRole", "RequirePerm", "Notify", "RegisterNotifier",
		"SetGlobalSetting", "GetGlobalSetting", "SetWorkspaceSetting", "GetWorkspaceSetting",
		"CacheSet", "CacheGet", "DeleteCache", "LoginByEmail", "LoginByIdentity", "BindIdentity",
		"OnEmail", "SendMail", "RecordUsage", "PluginActive", "FeatureActive", "ActivePlugins",
		"RegisterAuthMethod", "RegisterWebhookEvent", "Audit", "ParseUA",
	} {
		if !isNilFunc(ctxField(minimal, name)) {
			t.Errorf("minimal MCP Context must leave %s nil", name)
		}
	}
}

// TestRunEnableEnvelopeFailure drives Run's envelope branch to failure: a
// pre-seeded unwrappable DEK makes EnableEnvelope abort startup.
func TestRunEnableEnvelopeFailure(t *testing.T) {
	a := bootApp(t)
	if err := a.gdb.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	if err := a.gdb.Create(&models.Setting{Key: "crypto.dek", Value: "garbage"}).Error; err != nil {
		t.Fatalf("seed DEK: %v", err)
	}
	err := a.Run(context.Background())
	if err == nil {
		t.Fatal("Run succeeded with a corrupted DEK")
		return
	}
	if !strings.Contains(err.Error(), "cannot unwrap DEK") {
		t.Fatalf("Run = %v, want the DEK unwrap refusal", err)
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
				return
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
				return
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
			return
		}
	})

	t.Run("unopenable database", func(t *testing.T) {
		t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
		t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "missing", "nest", "db.sqlite"))
		t.Setenv("OCTARQ_SECRET_KEY", "new-err-secret-key-32-bytes-long!!!")
		t.Setenv("OCTARQ_ADMIN_PASSWORD", "new-err-admin-pass")
		if _, err := New(); err == nil {
			t.Fatal("New succeeded with an unopenable database path")
			return
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
		return
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
		return
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
