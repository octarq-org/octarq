package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/dns"
	"github.com/octarq-org/octarq/server/plugins/help"
	"github.com/octarq-org/octarq/server/plugins/links"
)

type echoIn struct {
	Message string `json:"message"`
}

type echoOut struct {
	Reply string `json:"reply"`
}

type appTestReactor struct{}

func (appTestReactor) Events() []string { return []string{"test.event"} }
func (appTestReactor) React(ctx context.Context, env plugin.Envelope) error {
	return nil
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
