// Package plugin defines the contract a commercial (Pro) module implements to
// extend octarq without forking it. It is the public, importable seam of the
// Core-as-Library split: the open-core binary depends only on this package and
// app; the private octarq-pro consumer registers plugins through it.
//
// AutoMigrate timing: a plugin contributes its GORM models via Models(). The
// app intentionally does NOT migrate at db-open time — it waits until every
// plugin has been registered, then runs AutoMigrate over the union of core and
// plugin models exactly once. A plugin therefore never has to (and must not)
// call AutoMigrate itself; doing so early would race the core schema.
//
// # Optional capabilities and compile-time assertions
//
// Beyond the required Plugin interface, a plugin opts into extra capabilities
// by implementing optional interfaces (Starter, MenuProvider, MCPProvider,
// OpenAPIContributor, Describer). The app detects them via runtime type
// assertion, which means a typo'd method name or a drifted signature does NOT
// fail the build — the capability is silently never invoked. To catch that at
// compile time, every plugin MUST pair each optional interface it implements
// with a compile-time assertion:
//
//	var (
//		_ plugin.Plugin       = (*Plugin)(nil)
//		_ plugin.Starter      = (*Plugin)(nil)
//		_ plugin.MenuProvider = (*Plugin)(nil)
//	)
//
// (use the value form `Plugin{}` if the plugin uses value receivers). See
// examples/plugin-hello for the canonical shape.
//
// # Inter-plugin services
//
// Two separately-compiled plugins interact through the service registry on
// Context — Provide and Lookup — never by importing each other. A provider
// registers a service value during Mount under a stable string name; the
// naming convention is "<pluginName>.<service>" (e.g. "billing.issuer",
// "hello.greeter"). The service value should be an interface defined in an
// importable package (the provider's own, or a shared contract package) so a
// consumer can retrieve it with the typed helper LookupAs.
//
// Lifecycle rules:
//
//   - Mount runs once per plugin, in registration (app.Use) order, on a
//     single goroutine. Provide must only be called during Mount.
//   - Lookup during Mount only sees services from plugins registered earlier.
//     Registration order is an app-wiring detail, so a cross-plugin consumer
//     must resolve lazily — in Start (the app launches Start goroutines only
//     after every plugin has mounted) or per-request — and degrade gracefully
//     when the name is absent (the provider may not be in this build).
//   - Providing the same name twice is a startup error: app.Run and
//     app.RunMCP refuse to serve.
//   - After the mount phase the registry is effectively read-only and safe
//     for concurrent Lookup from any goroutine.
//
// See Registry for the backing implementation and LookupAs for the canonical
// consumer shape.
//
// # Context evolution policy
package plugin

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/llmprovider"
	"gorm.io/gorm"
)

// ServiceDNSManager is the well-known service name under which the DNS manager is provided.
const ServiceDNSManager = "dns.manager"

type contextKey string

const orgIDKey contextKey = "org_id"

// WithOrgID returns a new context containing the organization ID for MCP / request scopes.
func WithOrgID(ctx context.Context, orgID uint) context.Context {
	return context.WithValue(ctx, orgIDKey, orgID)
}

// OrgIDFromContext extracts the organization ID from the context (0 if unset).
func OrgIDFromContext(ctx context.Context) uint {
	if id, ok := ctx.Value(orgIDKey).(uint); ok {
		return id
	}
	return 0
}

// EmailEvent is a stable, external snapshot of a freshly received inbound email,
// delivered to handlers registered via Context.OnEmail. It mirrors only the
// fields a plugin needs so plugins never import octarq's internal/models. The full
// row (including Raw RFC822 bytes, e.g. for attachment OCR) remains reachable
// via the shared DB using ID.
//
// This is the low-latency entry point Inbox AI needs (summary/classification,
// OTP extraction): the core fires it the moment an email is stored, instead of
// a plugin having to poll for unsummarized rows.
type EmailEvent struct {
	ID         uint      // emails.id — load the full row from the DB if more is needed
	MailboxID  uint      // owning mailbox
	OrgID      uint      // tenant scope (mailbox owner) for org-scoped processing
	From       string    // envelope/header From
	To         string    // recipient the mail was routed to
	Subject    string    // header Subject
	Text       string    // plaintext body (may be empty if HTML-only)
	HTML       string    // HTML body (may be empty)
	ReceivedAt time.Time // when the MTA received it
}

// Context carries the shared dependencies a plugin needs to wire its routes.
// It exposes only stable, external types so plugins in a separate module never
// reach into octarq's internal packages.
type Context struct {
	// Huma is the shared Huma API instance.
	//
	// Public (self-authenticated) routes: the core dashboard-auth middleware
	// 401s every /api/ operation that isn't in its hardcoded core allowlist.
	// A plugin route that must be reachable without a dashboard session — a
	// buyer-facing endpoint that checks its own buyer-session cookie, or an
	// intentionally public one — opts out by setting the boolean metadata key
	// "public" to true on its huma.Operation:
	//
	//	huma.Register(ctx.Huma, huma.Operation{
	//		Method: "POST", Path: "/api/customer/login",
	//		Metadata: map[string]any{"public": true},
	//	}, handler)
	//
	// The middleware skips ONLY the dashboard-auth check for that exact
	// operation — such a handler is responsible for authenticating its own
	// callers. This is exact, per-operation opt-in; it cannot widen to sibling
	// routes the way a path-prefix allowlist would. OPERATOR routes (anything
	// acting on operator/tenant-admin data) must NEVER be marked public.
	Huma huma.API
	// DB is the shared GORM handle. By the time Mount is called the plugin's
	// own Models() have already been migrated.
	DB *gorm.DB
	// Guard wraps a handler so it requires an authenticated dashboard session,
	// the same gate core endpoints use.
	Guard func(http.Handler) http.Handler
	// Notify delivers a notification via a configured channel. typ is the
	// channel type ("telegram", "webhook"), cfgJSON is the channel's JSON
	// config blob, and text is the message body. It mirrors notify.Send so
	// plugins never import octarq's internal/notify package directly.
	Notify func(ctx context.Context, typ, cfgJSON, text string) error
	// UserID extracts the authenticated user ID from the request session (0 if unauthed).
	UserID func(*http.Request) uint
	// OrgID extracts the authenticated org ID from the request session (0 if unauthed).
	OrgID func(*http.Request) uint
	// Audit writes an audit log entry asynchronously. action follows the
	// "resource.verb" convention (e.g. "subscription.create"). targetType is
	// the resource name, targetID is its primary key, meta is optional JSON
	// context (pass nil to omit). Mirrors the core h.audit() helper so plugins
	// never import octarq's internal/api or internal/models directly.
	Audit func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	// Encrypt seals plaintext with AES-256-GCM and returns base64(nonce||ciphertext).
	Encrypt func(plaintext []byte) (string, error)
	// Decrypt reverses Encrypt.
	Decrypt func(encoded string) ([]byte, error)
	// OnEmail registers a handler invoked (asynchronously, in its own goroutine)
	// after each inbound email is stored. Multiple plugins may register; a
	// handler must not block the request path and should bound its own work with
	// the context it captures. This is the inbound hook Inbox AI subscribes to.
	OnEmail func(handler func(EmailEvent))
	// DNS manages DNS records for a domain through the core's configured provider
	// (Cloudflare, …) so a plugin can change real records without importing octarq's
	// internal/dnsprovider. This is what makes "point the A record at a new IP"
	// an actual operation rather than a flag flip.
	DNS DNSManager
	// SendMail sends a transactional email through the org's configured SMTP
	// sender (the first mailmodels.SMTPSender for that org). Returns an error if the
	// org has no sender configured. Plugins use it for verification / password
	// reset without importing octarq's internal packages.
	SendMail func(orgID uint, to, subject, htmlBody, textBody string) error
	// SetLLMResolver replaces the LLM backend behind the core's single-step AI
	// assists (/api/ai/assist/*). The core's default resolver reads the OCTARQ_LLM_*
	// environment; the Pro ai plugin injects its DB-backed (dashboard-configured)
	// provider here so the assists follow the exact same configuration as Inbox
	// AI. The resolver runs on every assist request and must therefore be cheap —
	// cache internally and return an error describing how to configure when no
	// backend is usable.
	SetLLMResolver func(resolver func() (llmprovider.Provider, error))
	// Provide registers a service for other plugins to Lookup, under a stable
	// name following the "<pluginName>.<service>" convention. Call it only
	// during Mount. Providing a name twice is a startup error (the app refuses
	// to serve). See the "Inter-plugin services" section of the package doc.
	Provide func(name string, svc any)
	// Lookup resolves a service registered via Provide. During Mount it only
	// sees plugins mounted earlier, so cross-plugin consumers must call it
	// lazily — in Start or per-request. Safe for concurrent use after the
	// mount phase. Prefer the typed helper LookupAs.
	Lookup func(name string) (any, bool)
	// GetWorkspaceSetting reads a per-org setting value.
	GetWorkspaceSetting func(orgID uint, key string) string
	GetGlobalSetting    func(key string) string
	// SetWorkspaceSetting writes a per-org setting value.
	SetWorkspaceSetting func(orgID uint, key, value string) error
	// Enqueue adds a task to the background job queue.
	Enqueue func(ctx context.Context, taskType string, payload []byte) error
	// RegisterTask registers a handler for a task type in the background job queue.
	RegisterTask func(taskType string, handler func(ctx context.Context, payload []byte) error)
	// CacheGet retrieves a key from the global cache.
	CacheGet func(ctx context.Context, key string, val any) bool
	// CacheSet sets a key in the global cache.
	CacheSet func(ctx context.Context, key string, val any, ttl time.Duration) error
	// DeleteCache removes a key from the global cache.
	DeleteCache func(ctx context.Context, key string) error
	// GeoLookup resolves an IP address to country, region, city.
	GeoLookup func(ip string) (country, region, city string)
	// ParseUA parses a User-Agent string to device, browser, os.
	ParseUA func(ua string) (device, browser, os string)
	// PublishEvent publishes an event to the org's webhooks.
	PublishEvent func(orgID uint, event string, data any)
	// HandleRoot registers a handler on the core HTTP mux for the root path "/{slug}".
	HandleRoot func(handler http.Handler)
}

// DNSRecord is a provider-agnostic DNS record, mirroring the fields of octarq's
// internal dnsprovider.Record using only stable types so plugins in a separate
// module never import internal packages. An empty ID on a write means "create".
type DNSRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // A, AAAA, CNAME, TXT, MX, …
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Comment  string `json:"comment"`
	Priority *int   `json:"priority,omitempty"`
}

// DNSManager is the DNS-management seam exposed to plugins via Context.DNS. All
// operations take a octarq domain ID and resolve its zone + provider internally.
type DNSManager interface {
	// List returns all records in the domain's zone.
	List(ctx context.Context, domainID uint) ([]DNSRecord, error)
	// Set creates the record when r.ID is empty, otherwise updates it. Returns
	// the stored record.
	Set(ctx context.Context, domainID uint, r DNSRecord) (DNSRecord, error)
	// Delete removes a record by provider record ID.
	Delete(ctx context.Context, domainID uint, recordID string) error
}

// Mux is the subset of *http.ServeMux a plugin uses to register routes. The app
// passes a wrapper (not the raw mux) so it can gate every plugin route behind a
// per-workspace "plugin enabled" check without the plugin having to opt in.
// *http.ServeMux satisfies this interface, so plugin bodies are unchanged.
type Mux interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// Plugin is a unit of Pro functionality mounted onto the core app.
type Plugin interface {
	// Name is a short, stable identifier used in logs, the plugin registry, and
	// the per-workspace enable/disable setting (e.g. "ai", "infra", "billing").
	Name() string
	// Models returns the GORM models this plugin owns. They are collected and
	// migrated together with core models before any route is served.
	Models() []any
	// Mount registers the plugin's HTTP routes (typically under /api/...) on the
	// shared API mux. Use ctx.Guard to require a session. Every route registered
	// here is automatically gated: if the plugin is disabled for the caller's
	// workspace, the app answers 404 before the handler runs.
	Mount(mux Mux, ctx *Context)
}

// Starter is an optional interface a Plugin may implement. If present, the app
// calls Start in a goroutine after ALL plugins are mounted, passing the
// server's root context so the plugin can run background work (e.g. schedulers)
// and stop cleanly on shutdown. Because every Mount (and therefore every
// Provide) has completed by then, Start is the earliest safe point to Lookup
// services from other plugins regardless of registration order.
type Starter interface {
	Start(ctx context.Context)
}

// MenuItem represents a menu item exposed by a plugin for rendering in the UI.
type MenuItem struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`     // emoji or icon key
	Category string `json:"category"` // default category
}

// MenuProvider is an optional interface a Plugin may implement if it registers
// dynamic menu links for the frontend sidebar.
type MenuProvider interface {
	Menus() []MenuItem
}

// MCPProvider is an optional interface a Plugin may implement if it registers
// dynamic MCP tools.
type MCPProvider interface {
	RegisterMCP(srv *mcp.Server)
}

// OpenAPIContributor is an optional interface a Plugin may implement if it
// registers paths or schemas in the OpenAPI specification.
type OpenAPIContributor interface {
	OpenAPIPaths() map[string]any
	OpenAPISchemas() map[string]any
}

// Info is optional presentation/enablement metadata for a plugin. A plugin that
// does not implement Describer is treated as a standalone, user-toggleable
// feature keyed and titled by its Name().
type Info struct {
	// Title is the human label shown in the plugin manager (e.g. "Commerce").
	// Empty falls back to Name(), or, for a group, to the first member that
	// sets one.
	Title string
	// Group joins sibling plugins under a single toggle. Plugins sharing a Group
	// are enabled/disabled together as one feature; empty means the plugin is its
	// own feature. The enablement key is Group when set, otherwise Name().
	Group string
	// Core marks always-on plumbing (e.g. license activation, buyer identity):
	// never gated, not shown in the plugin manager, cannot be disabled.
	Core bool
	// Requires lists names of plugins that must be mounted for this plugin to
	// function. Validated at boot time.
	Requires []string
}

// Describer is the optional interface a Plugin implements to supply Info.
type Describer interface {
	Describe() Info
}

// Describe returns a plugin's Info, or a zero-value default (all fields empty).
func Describe(p Plugin) Info {
	if d, ok := p.(Describer); ok {
		return d.Describe()
	}
	return Info{}
}

// FeatureKey is the enable/disable unit for a plugin: its group if set, else its
// name. Plugins in the same group share one key and toggle together.
func FeatureKey(p Plugin) string {
	if g := Describe(p).Group; g != "" {
		return g
	}
	return p.Name()
}
