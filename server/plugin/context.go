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

func WithOrgID(ctx context.Context, orgID uint) context.Context {
	return tenantsql.WithOrgID(ctx, orgID)
}

func OrgIDFromContext(ctx context.Context) uint {
	return tenantsql.OrgIDFromContext(ctx)
}

func WithUserID(ctx context.Context, uid uint) context.Context {
	return tenantsql.WithUserID(ctx, uid)
}

func UserIDFromContext(ctx context.Context) uint {
	return tenantsql.UserIDFromContext(ctx)
}

const (
	DefaultWorkScope      = tenantsql.DefaultWorkScope
	PersonalPrivateScope  = tenantsql.PersonalPrivateScope
	ScopeWildcardWork     = tenantsql.ScopeWildcardWork
	ScopeWildcardPersonal = tenantsql.ScopeWildcardPerson
)

func WithScopeKey(ctx context.Context, scopeKey string) context.Context {
	return tenantsql.WithScopeKey(ctx, scopeKey)
}

func ScopeKeyFromContext(ctx context.Context) string {
	return tenantsql.ScopeKeyFromContext(ctx)
}

func WithAllowedScopes(ctx context.Context, scopes ...string) context.Context {
	return tenantsql.WithAllowedScopes(ctx, scopes...)
}

func AllowedScopesFromContext(ctx context.Context) []string {
	return tenantsql.AllowedScopesFromContext(ctx)
}

func AllowsPersonalScope(ctx context.Context) bool {
	return tenantsql.AllowsPersonalScope(ctx)
}

func ScopeAllowed(ctx context.Context, targetScope string) bool {
	return tenantsql.ScopeAllowed(ctx, targetScope)
}

type agentContextKey string

const (
	agentFlagKey  agentContextKey = "token_is_agent"
	agentToolsKey agentContextKey = "token_allowed_tools"
	tokenRoleKey  agentContextKey = "token_role"
)

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

func IsAgentFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if v, ok := ctx.Value(agentFlagKey).(bool); ok {
		return v
	}
	return false
}

func AllowedToolsFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(agentToolsKey).([]string); ok {
		return v
	}
	return nil
}

func WithTokenRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, tokenRoleKey, role)
}

func RoleFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(tokenRoleKey).(string); ok {
		return v
	}
	return ""
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

type Context struct {
	Huma          huma.API
	DB            *gorm.DB
	Guard         func(http.Handler) http.Handler
	Emit          func(ctx context.Context, payload NotificationPayload) error
	EmitTo        func(ctx context.Context, recipient NotificationRecipient, payload NotificationPayload) error
	FeatureActive func(orgID uint, featureKey string) bool
	PersonalOrgID func(userID uint) uint

	LoginByEmail                func(w http.ResponseWriter, r *http.Request, email string) (userID uint, err error)
	LoginByIdentity             func(w http.ResponseWriter, r *http.Request, id ExternalIdentity) (userID uint, err error)
	BindIdentity                func(r *http.Request, id ExternalIdentity) error
	RegisterAuthMethod          func(m AuthMethod)
	Audit                       func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	AuthenticateRequest         func(r *http.Request) (*http.Request, bool)
	SetLLMResolverForOrg        func(resolver func(orgID uint) (llmprovider.Provider, error))
	LLMProviderForOrg           func(orgID uint) (llmprovider.Provider, error)
	Provide                     func(name string, svc any)
	Lookup                      func(name string) (any, bool)
	Enqueue                     func(ctx context.Context, taskType string, payload []byte) error
	RegisterTask                func(taskType string, handler func(ctx context.Context, payload []byte) error)
	CacheGet                    func(ctx context.Context, key string, val any) bool
	CacheSet                    func(ctx context.Context, key string, val any, ttl time.Duration) error
	DeleteCache                 func(ctx context.Context, key string) error
	Cache                       ScopedCache
	GeoLookup                   func(ip string) (country, region, city string)
	ParseUA                     func(ua string) (device, browser, os string)
	RecordUsage                 func(orgID uint, metric string, n int64)
	HandleRoot                  func(handler http.Handler)
	HandleStatic                func(prefix string, fsys fs.FS)
	PluginActive                func(orgID uint, p Plugin) bool
	ActivePlugins               func() []Plugin
	Tracer                      func(name string) trace.Tracer
	Meter                       func(name string) metric.Meter
	StartSpan                   func(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
	RegisterEndpoint            func(spec any) error
	RegisterTenantView          func(view TenantView)
	RegisterReactor             func(r EventReactor) error
	RegisterNotificationChannel func(ch NotificationChannel)
	RegisterCron                func(name string, spec string, handler func(ctx context.Context) error) error
	Cron                        CronService
	Host                        Host
	TenantDB                    func(orgID uint) *TenantDB
}

func (c *Context) TenantDBFor(orgID uint) *TenantDB {
	if c != nil && c.TenantDB != nil {
		return c.TenantDB(orgID)
	}
	return nil
}

func (c *Context) PersonalOrgFor(userID uint) uint {
	if c != nil && c.PersonalOrgID != nil {
		return c.PersonalOrgID(userID)
	}
	return 0
}
