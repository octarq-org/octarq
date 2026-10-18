package plugin

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/llmprovider"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// WithOrgID returns a new context containing the organization ID for MCP / request scopes.
func WithOrgID(ctx context.Context, orgID uint) context.Context {
	return tenantsql.WithOrgID(ctx, orgID)
}

// OrgIDFromContext extracts the organization ID from the context (0 if unset).
func OrgIDFromContext(ctx context.Context) uint {
	return tenantsql.OrgIDFromContext(ctx)
}

// WithUserID returns a new context containing the authenticated user ID.
func WithUserID(ctx context.Context, uid uint) context.Context {
	return tenantsql.WithUserID(ctx, uid)
}

// UserIDFromContext extracts the authenticated user ID from context (0 if unset).
func UserIDFromContext(ctx context.Context) uint {
	return tenantsql.UserIDFromContext(ctx)
}

type agentContextKey string

const (
	agentFlagKey  agentContextKey = "token_is_agent"
	agentToolsKey agentContextKey = "token_allowed_tools"
)

// WithTokenAgent returns a new context containing the token's agent flag and allowed tools.
func WithTokenAgent(ctx context.Context, isAgent bool, allowedTools any) context.Context {
	var list []string
	switch v := allowedTools.(type) {
	case string:
		list = parseAllowedTools(v)
	case []string:
		list = v
	}
	ctx = context.WithValue(ctx, agentFlagKey, isAgent)
	return context.WithValue(ctx, agentToolsKey, list)
}

// IsAgentFromContext extracts the token agent flag from ctx.
func IsAgentFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if v, ok := ctx.Value(agentFlagKey).(bool); ok {
		return v
	}
	return false
}

// AllowedToolsFromContext extracts the permitted MCP tool names from ctx.
func AllowedToolsFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(agentToolsKey).([]string); ok {
		return v
	}
	return nil
}

func parseAllowedTools(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var list []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			var out []string
			for _, item := range list {
				trimmed := strings.TrimSpace(item)
				if trimmed != "" {
					out = append(out, trimmed)
				}
			}
			return out
		}
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
	// Emit delivers an event-driven notification through the notification router,
	// routing to recipients with user preferences, audit logging, and retry dispatch.
	// Preferred over Notify.
	Emit func(ctx context.Context, payload NotificationPayload) error
	// EmitTo delivers an event-driven notification directly to a recipient.
	EmitTo func(ctx context.Context, recipient NotificationRecipient, payload NotificationPayload) error
	// RegisterNotifier adds a notification channel provider under a type name,
	// letting a plugin contribute a new channel type (e.g. "slack", "sms") that
	// core event dispatch, the Notify hook, and the dashboard's channel test all
	// deliver to. cfgJSON is the channel's stored JSON config; text is the body.
	// Call it during Mount. nil on hosts that predate it.
	RegisterNotifier func(typ string, send func(ctx context.Context, cfgJSON, text string) error)
	// FeatureActive reports whether the given feature key is active for orgID.
	FeatureActive func(orgID uint, featureKey string) bool

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
	// LoginByIdentity completes a login for an externally verified identity,
	// keyed on (provider, issuer, subject) rather than on the asserted email.
	// Callers MUST have verified the assertion; this performs no authentication
	// of its own. It is what a per-org SSO plugin calls instead of LoginByEmail:
	// once each tenant supplies its own issuer and client credentials, the email
	// claim is attacker-controlled and matching on it is account takeover.
	// See ExternalIdentity for the full decision table. nil on hosts that
	// predate it.
	LoginByIdentity func(w http.ResponseWriter, r *http.Request, id ExternalIdentity) (userID uint, err error)
	// BindIdentity attaches an externally verified identity to the user already
	// signed in on this request, and is the only way an identity whose email
	// already has an account gets bound (ErrAccountLinkRequired). The direction
	// matters: the owner of the account initiates it from account settings while
	// authenticated, so controlling an IdP that asserts someone's address is not
	// enough to reach their account. Returns an error if nobody is signed in or
	// the identity is already bound to a different user. nil on hosts that
	// predate it.
	BindIdentity func(r *http.Request, id ExternalIdentity) error
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
	// SetLLMResolverForOrg replaces the LLM backend behind the core's single-step AI
	// assists (/api/ai/assist/*) with an org-aware resolver. A hosted instance serves
	// many tenants from one process, so the backend (and its API key) must be selected
	// per request rather than once per process. The core's default resolver reads the
	// OCTARQ_LLM_* environment; a downstream ai plugin injects its DB-backed
	// (dashboard-configured) provider here so the assists follow the exact same
	// configuration as Inbox AI. The resolver runs on every assist request and must
	// therefore be cheap — cache internally and return an error describing how to
	// configure when no backend is usable.
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
	// Cache provides plugin-scoped caching with automatic namespace isolation.
	// In standalone/single-binary builds, it uses in-memory LRU caching; when Redis
	// is configured, it transparently uses Redis.
	// nil on hosts that predate it.
	Cache ScopedCache
	// GeoLookup resolves an IP address to country, region, city.
	GeoLookup func(ip string) (country, region, city string)
	// ParseUA parses a User-Agent string to device, browser, os.
	ParseUA func(ua string) (device, browser, os string)
	// RecordUsage reports metered tenant consumption (a link redirect, an email
	// send) to whatever billing backend is installed. It is a no-op on
	// self-hosted builds where nothing provides "cloud.usage" — metering is a
	// hosted-only concern, so call sites must not branch on build or config.
	// Never expose this to tenant-controlled input: n is server-side truth.
	RecordUsage func(orgID uint, metric string, n int64)
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
	// ActivePlugins returns the currently registered plugins.
	ActivePlugins func() []Plugin
	// Tracer returns an OpenTelemetry Tracer for the plugin. nil on hosts that predate it.
	Tracer func(name string) trace.Tracer
	// Meter returns an OpenTelemetry Meter for the plugin. nil on hosts that predate it.
	Meter func(name string) metric.Meter
	// StartSpan starts an OpenTelemetry span for the plugin. nil on hosts that predate it.
	StartSpan func(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
	// RegisterEndpoint registers a declarative dual endpoint (HTTP + MCP).
	// nil on hosts that predate it.
	RegisterEndpoint func(spec any) error
	// RegisterTenantView registers a tenant-isolated SQL view.
	// Call it during Mount. nil on hosts that predate it.
	RegisterTenantView func(view TenantView)
	// RegisterReactor registers a declarative EventReactor to the event spine.
	// Call it during Mount. nil on hosts that predate it.
	RegisterReactor func(r EventReactor) error
	// RegisterNotificationChannel registers a notification channel SPI driver.
	// Call it during Mount. nil on hosts that predate it.
	RegisterNotificationChannel func(ch NotificationChannel)
	// RegisterCron registers a scheduled cron task.
	// nil on hosts that predate it.
	RegisterCron func(name string, spec string, handler func(ctx context.Context) error) error
	// Cron provides the CronService SPI for registering and listing cron tasks.
	// nil on hosts that predate it.
	Cron CronService
	// Host provides cohesive access to host runtime capabilities (Session, Crypto, Settings, Events).
	// nil on hosts that predate it.
	Host Host
	// TenantDB returns a type-safe, fail-closed database handle strictly scoped to orgID.
	// Returns nil if DB is not configured or orgID is 0. nil on hosts that predate it.
	TenantDB func(orgID uint) *TenantDB
}

// TenantDBFor returns a TenantDB scoped to orgID, or nil if unconfigured or orgID is 0.
func (c *Context) TenantDBFor(orgID uint) *TenantDB {
	if c != nil && c.TenantDB != nil {
		return c.TenantDB(orgID)
	}
	return nil
}
