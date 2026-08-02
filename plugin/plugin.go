// Package plugin defines the public, importable contract that anyone implements to
// extend octarq without forking it.
//
// Octarq's own core features (links, mail, domains) and the commercial Pro
// edition are built on this exact same seam — a community plugin can do
// everything an official plugin can do. The open-core binary depends only on
// this package and app; custom binaries and commercial builds register plugins
// through it.
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
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/llmprovider"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

// ErrLoginRegistrationDisabled is returned by Context.LoginByEmail when the
// email is unknown and the instance has public registration turned off
// (invite-only). An SSO/identity plugin should surface this as "ask an admin to
// add your account" rather than a generic error. It mirrors the core auth
// sentinel across the plugin boundary so plugins can errors.Is against it
// without importing internal packages.
var ErrLoginRegistrationDisabled = errors.New("registration disabled")

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

// WebhookEventDef describes a webhook event a plugin can fire via
// Context.PublishEvent, so the dashboard's webhook editor can offer it for
// subscription. It mirrors the core registry's definition using only stable
// types so plugins in a separate module never import octarq's internal packages.
type WebhookEventDef struct {
	Key         string // e.g. "link.click"
	Group       string // display group, e.g. "Links"
	Title       string // e.g. "Link Clicked"
	Description string // when the event fires
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
	// RegisterNotifier adds a notification channel provider under a type name,
	// letting a plugin contribute a new channel type (e.g. "slack", "sms") that
	// core event dispatch, the Notify hook, and the dashboard's channel test all
	// deliver to. cfgJSON is the channel's stored JSON config; text is the body.
	// Call it during Mount. nil on hosts that predate it.
	RegisterNotifier func(typ string, send func(ctx context.Context, cfgJSON, text string) error)
	// FeatureActive reports whether the given feature key is active for orgID.
	FeatureActive func(orgID uint, featureKey string) bool

	// UserID extracts the authenticated user ID from the request session (0 if unauthed).
	UserID func(*http.Request) uint
	// OrgID extracts the authenticated org ID from the request session (0 if unauthed).
	OrgID func(*http.Request) uint
	// OrgRole returns the role the caller holds in their ACTIVE org — "owner",
	// "admin", "member", or "" when unauthenticated, not a member, or
	// authenticated by API bearer token (which carries no user identity). Use it
	// to gate workspace-level administration: a plugin writing org-scoped config
	// should require owner/admin rather than merely "logged in".
	//
	// Authorization for a WORKSPACE-scoped resource. For instance-wide state use
	// IsInstanceAdmin instead — org role says nothing about instance privilege,
	// and every self-serve signup is "owner" of their own org.
	OrgRole func(*http.Request) string
	// RequireRole reports whether the caller holds at least the given workspace
	// role, for plugins gating destructive or credential-bearing operations.
	// A caller with no membership holds no role and is always refused.
	RequireRole func(r *http.Request, min string) bool
	// IsInstanceAdmin reports whether the caller is the bootstrap operator
	// account (User.IsInstanceAdmin, set deterministically for the configured
	// OCTARQ_ADMIN_* identity at first login — never derived from org ordering).
	//
	// This is the ONLY correct gate for instance-wide state: SSO/OIDC config, the
	// Pro license, instance branding defaults, anything one tenant must not be
	// able to change for every other tenant. "Is logged in" is NOT a substitute —
	// on a multi-tenant host every tenant is logged in.
	IsInstanceAdmin func(*http.Request) bool
	// LoginByEmail completes a login for an already-verified email address: it
	// provisions (or finds) the user + a personal org and issues the session
	// cookie, the same way built-in OAuth login does, returning the user ID. It
	// is the hook an SSO / identity plugin calls AFTER it has independently
	// verified the external identity (OIDC ID token, SAML assertion, …) — this
	// performs no authentication itself, so callers MUST verify first. It honours
	// the instance registration policy (an unknown email on an invite-only
	// instance is refused). A privileged capability: only compile-time-composed
	// plugins can reach it. nil on hosts that predate it.
	LoginByEmail func(w http.ResponseWriter, r *http.Request, email string) (userID uint, err error)
	// RegisterAuthMethod registers an available external auth method (e.g. SSO)
	// so the login page can discover and display it. SSO / identity plugins call
	// this during Mount. nil on hosts that predate it.
	RegisterAuthMethod func(m AuthMethod)
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
	// Deprecated: SetLLMResolver cannot express tenancy because its callback has no
	// orgID parameter. Plugins built against newer cores should prefer SetLLMResolverForOrg.
	SetLLMResolver func(resolver func() (llmprovider.Provider, error))
	// SetLLMResolverForOrg replaces the LLM backend behind the core's AI assists
	// with an org-aware resolver. A hosted instance serves many tenants from one
	// process, so the backend (and its API key) must be selected per request
	// rather than once per process. Prefer this over SetLLMResolver, which cannot
	// express tenancy and remains only for plugins built against older cores.
	SetLLMResolverForOrg func(resolver func(orgID uint) (llmprovider.Provider, error))
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
	// RecordUsage reports metered tenant consumption (a link redirect, an email
	// send) to whatever billing backend is installed. It is a no-op on
	// self-hosted builds where nothing provides "cloud.usage" — metering is a
	// hosted-only concern, so call sites must not branch on build or config.
	// Never expose this to tenant-controlled input: n is server-side truth.
	RecordUsage func(orgID uint, metric string, n int64)
	// RegisterWebhookEvent declares an event this plugin publishes via
	// PublishEvent so the dashboard's webhook editor can offer it for
	// subscription. Call it during Mount; duplicate keys are ignored.
	RegisterWebhookEvent func(def WebhookEventDef)
	// HandleRoot registers a handler on the core HTTP mux for the root path "/{slug}".
	HandleRoot func(handler http.Handler)
	// HandleStatic mounts an embedded single-page app under prefix (e.g.
	// "/portal"). fsys is the built dist directory and MUST contain an
	// index.html; requests under prefix serve a matching asset if one exists
	// and otherwise fall back to index.html so client-side routing works. This
	// is the seam a plugin uses to ship a self-contained buyer-facing frontend
	// (the buyer portal) without the core embedding it — the OSS build, which
	// composes no such plugin, simply 404s the prefix. Call it during Mount;
	// prefix should have no trailing slash. Mirrors HandleRoot for static SPAs.
	HandleStatic func(prefix string, fsys fs.FS)
	// PluginActive reports whether a plugin's feature is enabled for the
	// given workspace. Core plugins are always active.
	PluginActive func(orgID uint, p Plugin) bool
	// ActivePlugins returns a snapshot of all registered plugins.
	ActivePlugins func() []Plugin
}

// AuthMethod is a provider-agnostic auth method definition, mirroring the fields
// of internal/auth.AuthMethod using only stable types so plugins in a separate
// module never import internal packages.
type AuthMethod struct {
	ID       string `json:"id"`       // stable identifier, e.g. "sso" / "oidc-acme"
	Label    string `json:"label"`    // button label, e.g. "Sign in with SSO"
	LoginURL string `json:"loginUrl"` // launch/redirect URL (plugin's /api/... endpoint)
	IconKey  string `json:"iconKey"`  // frontend icon key, optional
	// Available reports whether the method can actually be used right now.
	// Registration happens at Mount, before any configuration exists, and there
	// is no way to unregister — so without this a method is offered from the
	// moment its plugin is compiled in. That is how the login page came to show
	// "Sign in with SSO" on instances where SSO was never configured: clicking
	// it reached a handler that refused with a bare 404 page.
	//
	// Consulted per request, so it tracks configuration changes. nil means
	// always available, which keeps existing plugins working unchanged.
	Available func() bool `json:"-"`
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
//
// Every operation also takes the org whose behalf it acts on, and resolves the
// domain scoped to that org: a domain id alone is a bare primary key, and these
// calls write into a real zone with that domain's stored provider credentials.
// Passing the caller's own org is what keeps one tenant out of another's DNS
// even when the calling plugin forgets to check ownership itself. orgID is
// required — 0 is rejected rather than treated as "any org".
type DNSManager interface {
	// List returns all records in the domain's zone.
	List(ctx context.Context, orgID, domainID uint) ([]DNSRecord, error)
	// Set creates the record when r.ID is empty, otherwise updates it. Returns
	// the stored record.
	Set(ctx context.Context, orgID, domainID uint, r DNSRecord) (DNSRecord, error)
	// Delete removes a record by provider record ID.
	Delete(ctx context.Context, orgID, domainID uint, recordID string) error
}

// LinkCreator is the short-link creation seam the links plugin publishes under
// the service name "links.create" (resolve it lazily with
// plugin.LookupAs[plugin.LinkCreator]). It lets any plugin turn a URL into a
// short link — the programmatic equivalent of POST /api/links — without
// importing the links plugin. It degrades gracefully: a consumer must tolerate
// the service being absent (links not composed into this build).
type LinkCreator interface {
	// CreateLink creates an enabled short link for targetURL in orgID's
	// workspace on the default host with a random slug, and returns the slug.
	// targetURL must be an http(s) URL.
	CreateLink(ctx context.Context, orgID uint, targetURL string) (slug string, err error)
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
	Order    int    `json:"order,omitempty"`
	// RequiredRole hides the entry from anyone below this org role
	// ("member" / "admin" / "owner"); empty shows it to everyone. It is a UX
	// hint only — the endpoints behind the page do their own enforcement, and
	// must, since nothing stops a caller from requesting the path directly.
	//
	// Set it whenever every route behind the menu entry is role-gated. Omitting
	// it there produces a nav item that leads only to a permission error, which
	// is how /roles behaved: visible to every member, 403 on arrival.
	RequiredRole string `json:"requiredRole,omitempty"`
}

// MenuProvider is an optional interface a Plugin may implement if it registers
// dynamic menu links for the frontend sidebar.
type MenuProvider interface {
	Menus() []MenuItem
}

// MCPProvider is an optional interface a Plugin may implement if it registers
// dynamic MCP tools. MCP tools MUST use plugin.OrgIDFromContext(ctx) to retrieve
// the caller's org ID, and MUST NOT read org fields cached on the plugin during Mount
// (under networked transport that field belongs to another caller).
type MCPProvider interface {
	RegisterMCP(srv *mcp.Server)
}

// HelpCategory is a sidebar group of help docs. The set is closed and lives
// here so a doc only has to name its category — labels never live in the shell.
type HelpCategory struct {
	Key    string            `json:"key"`    // "start", "access", …
	Order  int               `json:"order"`  // group ordering
	Icon   string            `json:"icon"`   // lucide key, e.g. "book-open"
	Labels map[string]string `json:"labels"` // lang → label; "en" is required
}

// HelpCategories returns the closed set of 6 help categories.
func HelpCategories() []HelpCategory {
	return []HelpCategory{
		{
			Key:   "start",
			Order: 10,
			Icon:  "book-open",
			Labels: map[string]string{
				"en": "Start here",
				"zh": "入门",
				"es": "Primeros pasos",
				"pt": "Primeiros passos",
				"ja": "はじめに",
			},
		},
		{
			Key:   "access",
			Order: 20,
			Icon:  "shield",
			Labels: map[string]string{
				"en": "Access & identity",
				"zh": "身份与访问",
				"es": "Acceso e identidad",
				"pt": "Acesso e identidade",
				"ja": "アクセスと認証",
			},
		},
		{
			Key:   "automation",
			Order: 30,
			Icon:  "bot",
			Labels: map[string]string{
				"en": "Automation & APIs",
				"zh": "自动化与 API",
				"es": "Automatización y API",
				"pt": "Automação e APIs",
				"ja": "自動化と API",
			},
		},
		{
			Key:   "services",
			Order: 40,
			Icon:  "boxes",
			Labels: map[string]string{
				"en": "Services",
				"zh": "服务",
				"es": "Servicios",
				"pt": "Serviços",
				"ja": "サービス",
			},
		},
		{
			Key:   "commerce",
			Order: 50,
			Icon:  "credit-card",
			Labels: map[string]string{
				"en": "Commerce & billing",
				"zh": "商业化与计费",
				"es": "Comercio y facturación",
				"pt": "Comércio e faturamento",
				"ja": "商取引と請求",
			},
		},
		{
			Key:   "licensing",
			Order: 60,
			Icon:  "key",
			Labels: map[string]string{
				"en": "Editions & licensing",
				"zh": "版本与授权",
				"es": "Ediciones y licencias",
				"pt": "Edições e licenças",
				"ja": "エディションとライセンス",
			},
		},
	}
}

var helpCategoryMap = func() map[string]HelpCategory {
	m := make(map[string]HelpCategory)
	for _, c := range HelpCategories() {
		m[c.Key] = c
	}
	return m
}()

// CompareHelpDocs orders HelpDocs first by category order, then doc Order, then Title.
func CompareHelpDocs(a, b HelpDoc) bool {
	catAOrder := 9999
	if c, ok := helpCategoryMap[a.Category]; ok {
		catAOrder = c.Order
	}
	catBOrder := 9999
	if c, ok := helpCategoryMap[b.Category]; ok {
		catBOrder = c.Order
	}

	if catAOrder != catBOrder {
		return catAOrder < catBOrder
	}
	if a.Order != b.Order {
		return a.Order < b.Order
	}
	return a.Title < b.Title
}

// HelpDocTranslation provides localized strings for a HelpDoc.
type HelpDocTranslation struct {
	Title    string `yaml:"title" json:"title,omitempty"`
	Category string `yaml:"category" json:"category,omitempty"`
	Markdown string `yaml:"markdown" json:"markdown,omitempty"`
}

// HelpDoc is one page of in-app documentation contributed by a plugin.
type HelpDoc struct {
	Slug         string                        `yaml:"slug" json:"slug"`
	Title        string                        `yaml:"title" json:"title"`
	Category     string                        `yaml:"category" json:"category"`
	Order        int                           `yaml:"order" json:"order"`
	Feature      string                        `yaml:"feature" json:"feature,omitempty"`
	Markdown     string                        `yaml:"-" json:"markdown"`
	Translations map[string]HelpDocTranslation `yaml:"translations" json:"translations,omitempty"`
}

// WithTranslation parses translation content (which can also contain YAML frontmatter or raw markdown)
// and adds/updates the translation for the specified language key (e.g. "zh").
func (h HelpDoc) WithTranslation(lang, raw string) HelpDoc {
	if h.Translations == nil {
		h.Translations = make(map[string]HelpDocTranslation)
	}
	trDoc, err := ParseHelpDoc(raw)
	tr := h.Translations[lang]
	if err == nil && (trDoc.Title != "" || trDoc.Category != "" || trDoc.Markdown != "") {
		if trDoc.Title != "" {
			tr.Title = trDoc.Title
		}
		if trDoc.Category != "" {
			tr.Category = trDoc.Category
		}
		tr.Markdown = trDoc.Markdown
	} else {
		tr.Markdown = strings.TrimSpace(raw)
	}
	h.Translations[lang] = tr
	return h
}

// ParseHelpDoc parses an MDX/MD string with optional YAML frontmatter.
func ParseHelpDoc(content string) (HelpDoc, error) {
	var doc HelpDoc
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		doc.Markdown = trimmed
		return doc, nil
	}

	rest := trimmed[3:]
	if idx := strings.Index(rest, "\n"); idx != -1 {
		rest = rest[idx+1:]
	}

	endIdx := strings.Index(rest, "---")
	if endIdx == -1 {
		doc.Markdown = trimmed
		return doc, nil
	}

	frontmatterYAML := rest[:endIdx]
	markdownBody := strings.TrimSpace(rest[endIdx+3:])

	if err := yaml.Unmarshal([]byte(frontmatterYAML), &doc); err != nil {
		return doc, err
	}
	doc.Markdown = markdownBody
	return doc, nil
}

// ParseHelpDocSafe parses an MDX/MD string and returns a fallback doc with markdown body on error without panic.
func ParseHelpDocSafe(content string) HelpDoc {
	doc, err := ParseHelpDoc(content)
	if err != nil {
		log.Printf("[help] warning: failed to parse help doc: %v", err)
		return HelpDoc{
			Markdown: strings.TrimSpace(content),
		}
	}
	return doc
}

// FillDefaults automatically populates Category based on taxonomy defaults if missing or invalid.
func (h *HelpDoc) FillDefaults(pluginName string, pluginCategory string) {
	if h.Category == "" {
		if pluginCategory != "" {
			h.Category = pluginCategory
		} else {
			h.Category = pluginName
		}
	}
	h.Category = strings.ToLower(h.Category)

	// Remap known legacy categories or validate against closed set
	switch h.Category {
	case "getting-started", "start", "general", "platform", "core":
		h.Category = "start"
	case "access", "security", "identity":
		h.Category = "access"
	case "automation", "api", "apis":
		h.Category = "automation"
	case "services", "infrastructure", "dns", "ddns", "short-links", "links", "mail", "messaging", "marketing":
		h.Category = "services"
	case "commerce", "billing", "finance":
		h.Category = "commerce"
	case "licensing", "editions":
		h.Category = "licensing"
	default:
		if _, ok := helpCategoryMap[h.Category]; !ok {
			log.Printf("[help] warning: doc %q from plugin %q has unknown category %q, falling back to 'services'", h.Slug, pluginName, h.Category)
			h.Category = "services"
		}
	}
}

// MustParseHelpDoc calls ParseHelpDoc and panics if an error occurs.
func MustParseHelpDoc(content string) HelpDoc {
	doc, err := ParseHelpDoc(content)
	if err != nil {
		panic(err)
	}
	return doc
}

// HelpProvider is implemented by plugins that ship in-app documentation.
type HelpProvider interface {
	HelpDocs() []HelpDoc
}

// OpenAPIContributor is an optional interface a Plugin may implement if it
// registers paths or schemas in the OpenAPI specification.
type OpenAPIContributor interface {
	OpenAPIPaths() map[string]any
	OpenAPISchemas() map[string]any
}

// Category constants for plugin classification.
const (
	CategoryMarketing      = "marketing"
	CategoryMessaging      = "messaging"
	CategorySecurity       = "security"
	CategoryInfrastructure = "infrastructure"
	CategoryCommerce       = "commerce"
	CategoryAI             = "ai"
	CategoryUtilities      = "utilities"
)

// ValidCategories is the set of allowed category constants for plugins.
var ValidCategories = map[string]bool{
	CategoryMarketing:      true,
	CategoryMessaging:      true,
	CategorySecurity:       true,
	CategoryInfrastructure: true,
	CategoryCommerce:       true,
	CategoryAI:             true,
	CategoryUtilities:      true,
}

// Info is optional presentation/enablement metadata for a plugin. A plugin that
// does not implement Describer is treated as a standalone, user-toggleable
// feature keyed and titled by its Name().
type Info struct {
	// Title is the human label shown in the plugin manager (e.g. "Commerce").
	// Empty falls back to Name(), or, for a group, to the first member that
	// sets one.
	Title string
	// Description is the summary of what this feature/plugin provides.
	Description string
	// Icon is the feature's card icon in the plugin manager: a Lucide icon key
	// (see PLUGIN_ICONS in web/src/shell/areas.tsx) or a logo image URL.
	//
	// Leave it empty when the plugin has a menu — the card falls back to that
	// menu's icon, so repeating it here is duplication that can drift. Set it
	// for a background service with no menu, or when the card should
	// deliberately differ from the sidebar.
	//
	// A string that isn't a known key is rendered literally, as text. That is
	// intentional (it lets a plugin ship an emoji) but it means a typo shows up
	// as copy rather than a broken image — web/src/shell/menuIcons.test.ts
	// guards the Go-declared menu icons against exactly that.
	Icon string
	// Category is the primary classification constant (CategoryMarketing, CategorySecurity, etc.).
	Category string
	// Tags are optional tags for extra sub-filtering (e.g. []string{"smtp", "webhook"}).
	Tags []string
	// Group joins sibling plugins under a single toggle. Plugins sharing a Group
	// are enabled/disabled together as one feature; empty means the plugin is its
	// own feature. The enablement key is Group when set, otherwise Name().
	Group string
	// Core marks always-on plumbing (e.g. license activation, buyer identity):
	// never gated, not shown in the plugin manager, cannot be disabled.
	Core bool
	// EnabledByDefault makes a (non-Core) feature active for a workspace that has
	// never toggled it — i.e. when no PluginSetting row exists. The feature stays
	// user-toggleable in the plugin manager; this only changes the pre-toggle
	// default from off (opt-in) to on (opt-out). Ignored when Core is set.
	EnabledByDefault bool
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

// FeatureIsCore reports whether the feature identified by key is always-on
// plumbing within the given plugin set.
//
// Core-ness belongs to the feature, not to a single member. Several plugins can
// share one feature key — an OSS half that serves the routes and a Pro half that
// only contributes content, say — and if either declares Core the feature as a
// whole is never gated. Deciding per plugin instead lets the halves disagree:
// the manager offers a toggle that the Core half ignores, so turning the feature
// "off" leaves its menu and routes live. Every enablement decision (route gate,
// menu filter, manager listing) goes through this.
func FeatureIsCore(plugins []Plugin, key string) bool {
	for _, p := range plugins {
		if FeatureKey(p) == key && Describe(p).Core {
			return true
		}
	}
	return false
}
