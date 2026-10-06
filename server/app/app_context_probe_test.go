package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/help"
)

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
	if ctx.Guard == nil {
		p.present("DB", ctx.DB != nil)
		p.present("Provide", ctx.Provide != nil)
		p.present("Lookup", ctx.Lookup != nil)
		return
	}

	// The whole surface must be wired.
	p.present("Guard", ctx.Guard != nil)
	p.present("Audit", ctx.Audit != nil)
	p.present("Notify", ctx.Notify != nil)
	p.present("RegisterNotifier", ctx.RegisterNotifier != nil)
	p.present("SendMail", ctx.SendMail != nil)
	p.present("RecordUsage", ctx.RecordUsage != nil)
	p.present("CacheGet", ctx.CacheGet != nil)
	p.present("CacheSet", ctx.CacheSet != nil)
	p.present("DeleteCache", ctx.DeleteCache != nil)
	p.present("Cache", ctx.Cache != nil)
	p.present("ParseUA", ctx.ParseUA != nil)
	p.present("RegisterAuthMethod", ctx.RegisterAuthMethod != nil)
	p.present("LoginByEmail", ctx.LoginByEmail != nil)
	p.present("LoginByIdentity", ctx.LoginByIdentity != nil)
	p.present("BindIdentity", ctx.BindIdentity != nil)
	p.present("Provide", ctx.Provide != nil)
	p.present("Lookup", ctx.Lookup != nil)
	p.present("DB", ctx.DB != nil)
	p.present("RegisterEndpoint", ctx.RegisterEndpoint != nil)
	p.present("RegisterTenantView", ctx.RegisterTenantView != nil)
	p.present("RegisterReactor", ctx.RegisterReactor != nil)
	p.present("Host", ctx.Host != nil)
	p.present("Host.Session", ctx.Host != nil && ctx.Host.Session() != nil)
	p.present("Host.Crypto", ctx.Host != nil && ctx.Host.Crypto() != nil)
	p.present("Host.Settings", ctx.Host != nil && ctx.Host.Settings() != nil)
	p.present("Host.Events", ctx.Host != nil && ctx.Host.Events() != nil)

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
	if err := ctx.Host.Settings().SetGlobalSetting("cov.g"+p.name, "gv"); err != nil {
		t.Fatalf("SetGlobalSetting: %v", err)
	}
	if got := ctx.Host.Settings().GetGlobalSetting("cov.g" + p.name); got != "gv" {
		t.Errorf("GetGlobalSetting after Set = %q, want gv", got)
	}

	// Workspace setting round-trip + cross-org isolation. A fixed org id far
	// from the auto-increment range keeps the isolation check unambiguous; the
	// key is per-probe so the two instances never overwrite each other.
	wsOrg := uint(4242)
	if err := ctx.Host.Settings().SetWorkspaceSetting(wsOrg, "cov.ws"+p.name, "wv"); err != nil {
		t.Fatalf("SetWorkspaceSetting: %v", err)
	}
	if got := ctx.Host.Settings().GetWorkspaceSetting(wsOrg, "cov.ws"+p.name); got != "wv" {
		t.Errorf("GetWorkspaceSetting after Set = %q, want wv", got)
	}
	if got := ctx.Host.Settings().GetWorkspaceSetting(wsOrg+1, "cov.ws"+p.name); got != "" {
		t.Errorf("GetWorkspaceSetting crossed org boundaries: %q", got)
	}

	// A registered notifier must receive the Notify.
	var gotText string
	ctx.RegisterNotifier("cov.notify."+p.name, func(_ context.Context, _, text string) error {
		gotText = text
		return nil
	})
	cfg, err := ctx.Host.Crypto().Encrypt([]byte(`{}`))
	if err != nil {
		t.Fatalf("Encrypt notifier config: %v", err)
	}
	if err := ctx.Notify(context.Background(), "cov.notify."+p.name, cfg, "hello "+p.name); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotText != "hello "+p.name {
		t.Errorf("notifier received %q, want %q", gotText, "hello "+p.name)
	}
	// ctx.Notify error on undecryptable config
	if err := ctx.Notify(context.Background(), "cov.notify."+p.name, "undecryptable-garbage", "hello"); err == nil {
		t.Errorf("expected ctx.Notify error on undecryptable config, got nil")
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
	if got := ctx.Host.Session().UserID(authedReq); got != u.ID {
		t.Errorf("UserID = %d, want %d", got, u.ID)
	}
	if got := ctx.Host.Session().OrgID(authedReq); got != o.ID {
		t.Errorf("OrgID = %d, want %d", got, o.ID)
	}
	if got := ctx.Host.Session().OrgRole(authedReq); got != "owner" {
		t.Errorf("OrgRole = %q, want owner", got)
	}
	if ctx.Host.Session().IsInstanceAdmin(authedReq) {
		t.Error("a regular user was reported instance admin")
	}
	if !ctx.Host.Session().RequireRole(authedReq, "owner") {
		t.Error("owner failed RequireRole(owner)")
	}
	if !ctx.Host.Session().RequirePerm(authedReq, "cov.perm", "member") {
		t.Error("owner failed RequirePerm(min member)")
	}

	// Encrypt/Decrypt round-trip.
	enc, err := ctx.Host.Crypto().Encrypt([]byte("cov secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if dec, err := ctx.Host.Crypto().Decrypt(enc); err != nil || string(dec) != "cov secret" {
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
	ctx.Host.Events().RegisterWebhookEvent(plugin.WebhookEventDef{Key: "cov.event." + p.name, Group: "cov", Title: "t", Description: "d"})
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
	ctx.Host.Events().OnEmail(func(plugin.EmailEvent) {})

	// Provide/Lookup round-trip.
	svc := "cov.svc." + p.name
	ctx.Provide(svc, "svcval")
	if v, ok := ctx.Lookup(svc); !ok || v != "svcval" {
		t.Errorf("Lookup after Provide = %v (ok=%v), want svcval", v, ok)
	}

	// Exercise Tracer, Meter, GeoLookup, Enqueue, RegisterTask, RegisterReactor, RegisterTenantView, RegisterEndpoint
	if ctx.Tracer != nil {
		_ = ctx.Tracer("test-tracer")
	}
	if ctx.Meter != nil {
		_ = ctx.Meter("test-meter")
	}
	if ctx.GeoLookup != nil {
		_, _, _ = ctx.GeoLookup("1.1.1.1")
	}
	if ctx.RegisterTask != nil {
		ctx.RegisterTask("test-task-"+p.name, func(ctx context.Context, payload []byte) error { return nil })
	}
	if ctx.Enqueue != nil {
		_ = ctx.Enqueue(context.Background(), "test-task-"+p.name, []byte("payload"))
	}
	if ctx.RegisterReactor != nil {
		_ = plugin.RegisterReactor(ctx, appTestReactor{})
	}
	if ctx.RegisterTenantView != nil {
		ctx.RegisterTenantView(plugin.TenantView{Name: "tenant_test_" + p.name})
	}
	if ctx.RegisterEndpoint != nil {
		_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[echoIn, echoOut]{
			Name:   "ep_" + p.name,
			Method: "POST",
			Path:   "/api/ep-" + p.name,
			Handler: func(ctx context.Context, in echoIn) (*echoOut, error) {
				return &echoOut{Reply: in.Message}, nil
			},
		})
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
	if n := ctx.Host.Session().RevokeUserOrgSessions(u.ID, o.ID); n < 1 {
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
