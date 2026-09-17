package app

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/api"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/cache"
	"github.com/octarq-org/octarq/server/internal/cron"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/endpoint"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/internal/notify"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"github.com/octarq-org/octarq/server/plugin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// hostRuntime implements plugin.Host.
type hostRuntime struct {
	session  plugin.HostSession
	crypto   plugin.CryptoVault
	settings plugin.SettingsStore
	events   plugin.EventSpine
	db       *gorm.DB
}

func (h *hostRuntime) Session() plugin.HostSession    { return h.session }
func (h *hostRuntime) Crypto() plugin.CryptoVault     { return h.crypto }
func (h *hostRuntime) Settings() plugin.SettingsStore { return h.settings }
func (h *hostRuntime) Events() plugin.EventSpine      { return h.events }
func (h *hostRuntime) TenantDB(orgID uint) *plugin.TenantDB {
	if h.db == nil || orgID == 0 {
		return nil
	}
	tdb, err := tenantsql.NewTenantDB(h.db, orgID)
	if err != nil {
		return nil
	}
	return tdb
}

var _ plugin.Host = (*hostRuntime)(nil)

// hostSession implements plugin.HostSession.
type hostSession struct {
	auth       *auth.Manager
	apiHandler *api.Handler
}

func (s *hostSession) UserID(r *http.Request) uint {
	if s.auth != nil {
		return s.auth.UserID(r)
	}
	return 0
}

func (s *hostSession) OrgID(r *http.Request) uint {
	if s.auth != nil {
		return s.auth.OrgID(r)
	}
	return 0
}

func (s *hostSession) OrgRole(r *http.Request) string {
	if s.apiHandler != nil {
		return s.apiHandler.OrgRole(r)
	}
	return ""
}

func (s *hostSession) RequireRole(r *http.Request, min string) bool {
	if s.apiHandler != nil {
		return s.apiHandler.RequireRole(r, min)
	}
	return false
}

func (s *hostSession) RequirePerm(r *http.Request, permKey, minRole string) bool {
	if s.apiHandler != nil {
		return s.apiHandler.RequirePerm(r, permKey, minRole)
	}
	return false
}

func (s *hostSession) IsInstanceAdmin(r *http.Request) bool {
	if s.apiHandler != nil {
		return s.apiHandler.IsInstanceAdmin(r)
	}
	return false
}

func (s *hostSession) RevokeUserOrgSessions(userID, orgID uint) int {
	if s.auth != nil {
		return s.auth.RevokeUserOrgSessions(userID, orgID)
	}
	return 0
}

var _ plugin.HostSession = (*hostSession)(nil)

// hostCryptoVault implements plugin.CryptoVault.
type hostCryptoVault struct {
	cipher *crypto.Cipher
}

func (c *hostCryptoVault) Encrypt(plaintext []byte) (string, error) {
	if c.cipher != nil {
		return c.cipher.Encrypt(plaintext)
	}
	return "", errors.New("cipher not configured")
}

func (c *hostCryptoVault) Decrypt(encoded string) ([]byte, error) {
	if c.cipher != nil {
		return c.cipher.Decrypt(encoded)
	}
	return nil, errors.New("cipher not configured")
}

var _ plugin.CryptoVault = (*hostCryptoVault)(nil)

// hostSettingsStore implements plugin.SettingsStore.
type hostSettingsStore struct {
	apiHandler *api.Handler
}

func (s *hostSettingsStore) GetWorkspaceSetting(orgID uint, key string) string {
	if s.apiHandler != nil {
		return s.apiHandler.GetWorkspaceSetting(orgID, key)
	}
	return ""
}

func (s *hostSettingsStore) SetWorkspaceSetting(orgID uint, key, value string) error {
	if s.apiHandler != nil {
		return s.apiHandler.SetWorkspaceSetting(orgID, key, value)
	}
	return errors.New("api handler not configured")
}

func (s *hostSettingsStore) GetGlobalSetting(key string) string {
	if s.apiHandler != nil {
		return s.apiHandler.GetGlobalSetting(key)
	}
	return ""
}

func (s *hostSettingsStore) SetGlobalSetting(key, value string) error {
	if s.apiHandler != nil {
		return s.apiHandler.SetGlobalSetting(key, value)
	}
	return errors.New("api handler not configured")
}

var _ plugin.SettingsStore = (*hostSettingsStore)(nil)

// hostEventSpine implements plugin.EventSpine.
type hostEventSpine struct {
	services        *plugin.Registry
	emailMu         *sync.Mutex
	deferredOnEmail *[]func(plugin.EmailEvent)
}

func (e *hostEventSpine) PublishEvent(orgID uint, event string, data any) {
	eventbus.Publish(orgID, event, data)
}

func (e *hostEventSpine) RegisterWebhookEvent(def plugin.WebhookEventDef) {
	eventbus.RegisterEventDef(eventbus.EventDef{
		Key:         def.Key,
		Group:       def.Group,
		Title:       def.Title,
		Description: def.Description,
	})
}

func (e *hostEventSpine) OnEmail(handler func(plugin.EmailEvent)) {
	if handler == nil {
		return
	}
	if e.services != nil {
		if onEmailService, ok := plugin.LookupServiceAs[plugin.EmailDispatcher](e.services.Lookup, plugin.ServiceMailDispatcher); ok {
			onEmailService(handler)
			return
		}
	}
	if e.emailMu != nil && e.deferredOnEmail != nil {
		e.emailMu.Lock()
		*e.deferredOnEmail = append(*e.deferredOnEmail, handler)
		e.emailMu.Unlock()
	}
}

var _ plugin.EventSpine = (*hostEventSpine)(nil)

// pluginContextParams collects dependencies needed to construct a plugin.Context.
type pluginContextParams struct {
	apiHandler      *api.Handler
	gdb             *gorm.DB
	cipher          *crypto.Cipher
	auth            *auth.Manager
	notifRouter     *notification.Router
	services        *plugin.Registry
	geo             *geo.Resolver
	endpointEngine  *endpoint.Engine
	taskQueue       queue.Queue
	cronEngine      *cron.Engine
	sendMail        func(orgID uint, to, subject, htmlBody, textBody string) error
	loginByEmail    func(w http.ResponseWriter, r *http.Request, email string) (uint, error)
	loginByIdentity func(w http.ResponseWriter, r *http.Request, id plugin.ExternalIdentity) (uint, error)
	onEmailMu       *sync.Mutex
	deferredOnEmail *[]func(plugin.EmailEvent)
	handleRoot      func(http.Handler)
	handleStatic    func(prefix string, fsys fs.FS)
	httpMode        bool
	plugins         []plugin.Plugin
}

// buildPluginContext constructs a unified plugin.Context with high-cohesion Host runtime capabilities
// and delegates legacy closure fields directly to the underlying host components.
func (a *App) buildPluginContext(params pluginContextParams) *plugin.Context {
	db := params.gdb
	if db == nil && a != nil {
		db = a.gdb
	}
	cipher := params.cipher
	if cipher == nil && a != nil {
		cipher = a.cipher
	}
	authMgr := params.auth
	if authMgr == nil && a != nil {
		authMgr = a.auth
	}
	geoResolver := params.geo
	if geoResolver == nil && a != nil {
		geoResolver = a.geo
	}
	pluginsList := params.plugins
	if pluginsList == nil && a != nil {
		pluginsList = a.plugins
	}
	sendMail := params.sendMail
	if sendMail == nil && a != nil {
		sendMail = a.sendMail
	}
	loginByEmail := params.loginByEmail
	if loginByEmail == nil && a != nil {
		loginByEmail = a.loginByEmail
	}
	loginByIdentity := params.loginByIdentity
	if loginByIdentity == nil && a != nil {
		loginByIdentity = a.loginByIdentity
	}

	session := &hostSession{
		auth:       authMgr,
		apiHandler: params.apiHandler,
	}
	vault := &hostCryptoVault{
		cipher: cipher,
	}
	settings := &hostSettingsStore{
		apiHandler: params.apiHandler,
	}
	events := &hostEventSpine{
		services:        params.services,
		emailMu:         params.onEmailMu,
		deferredOnEmail: params.deferredOnEmail,
	}

	host := &hostRuntime{
		session:  session,
		crypto:   vault,
		settings: settings,
		events:   events,
		db:       db,
	}

	var apiHuma huma.API
	var auditFn func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	if params.apiHandler != nil {
		apiHuma = params.apiHandler.Huma()
		auditFn = params.apiHandler.Audit
	}

	var authGuard func(http.Handler) http.Handler
	var bindIdentity func(r *http.Request, id plugin.ExternalIdentity) error
	var cacheGet func(ctx context.Context, key string, val any) bool
	var cacheSet func(ctx context.Context, key string, val any, ttl time.Duration) error
	var deleteCache func(ctx context.Context, key string) error
	var scopedCache plugin.ScopedCache

	if authMgr != nil {
		authGuard = authMgr.Require
		bindIdentity = authMgr.BindIdentity
		if authMgr.Cache() != nil {
			cacheGet = authMgr.Cache().Get
			cacheSet = authMgr.Cache().Set
			deleteCache = authMgr.Cache().Delete
			scopedCache = cache.NewScoped(authMgr.Cache(), "core")
		}
	}

	var servicesLookup func(name string) (any, bool)
	var servicesProvide func(name string, svc any)
	if params.services != nil {
		servicesLookup = params.services.Lookup
		servicesProvide = params.services.Provide
	}

	pctx := &plugin.Context{
		Host:     host,
		TenantDB: host.TenantDB,
		Huma:     apiHuma,
		DB:       db,
		Guard:    authGuard,
		Notify: func(ctx context.Context, typ, cfgJSON, text string) error {
			pt, err := notification.ConfigPlaintext(cfgJSON)
			if err != nil {
				return err
			}
			if params.notifRouter != nil {
				return params.notifRouter.SendDirect(ctx, typ, pt, text)
			}
			return notification.DefaultRouter().SendDirect(ctx, typ, pt, text)
		},
		Emit: func(ctx context.Context, payload plugin.NotificationPayload) error {
			if params.notifRouter != nil {
				return params.notifRouter.Emit(ctx, payload)
			}
			return notification.DefaultRouter().Emit(ctx, payload)
		},
		EmitTo: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			if params.notifRouter != nil {
				return params.notifRouter.EmitTo(ctx, recipient, payload)
			}
			return notification.DefaultRouter().EmitTo(ctx, recipient, payload)
		},
		RegisterNotificationChannel: func(ch plugin.NotificationChannel) {
			if params.notifRouter != nil {
				_ = params.notifRouter.RegisterChannel(ch)
			}
		},
		RegisterNotifier: func(typ string, send func(ctx context.Context, cfgJSON, text string) error) {
			if params.notifRouter != nil {
				_ = params.notifRouter.RegisterChannel(notification.NewLegacyNotifierAdapter(typ, "", send))
			} else {
				notify.Register(typ, send)
			}
		},
		RevokeUserOrgSessions: session.RevokeUserOrgSessions,
		UserID:                session.UserID,
		OrgID:                 session.OrgID,
		OrgRole:               session.OrgRole,
		RequireRole:           session.RequireRole,
		RequirePerm:           session.RequirePerm,
		IsInstanceAdmin:       session.IsInstanceAdmin,
		LoginByEmail:          loginByEmail,
		LoginByIdentity:       loginByIdentity,
		BindIdentity:          bindIdentity,
		RegisterAuthMethod: func(m plugin.AuthMethod) {
			auth.Register(auth.AuthMethod{
				ID:        m.ID,
				Label:     m.Label,
				LoginURL:  m.LoginURL,
				IconKey:   m.IconKey,
				Available: m.Available,
			})
		},
		Audit:    auditFn,
		Encrypt:  vault.Encrypt,
		Decrypt:  vault.Decrypt,
		OnEmail:  events.OnEmail,
		DNS:      &lazyDNSManager{lookup: servicesLookup},
		SendMail: sendMail,
		SetLLMResolverForOrg: func(resolver func(orgID uint) (llmprovider.Provider, error)) {
			if params.apiHandler != nil {
				params.apiHandler.SetLLMResolverForOrg(resolver)
			}
		},
		RecordUsage: func(orgID uint, metric string, n int64) {
			// Lazily resolved on every call: the provider (Pro's cloud module) may
			// mount after the plugin that meters, so a Mount-time Lookup would
			// silently never find it.
			if params.services != nil {
				if fn, ok := plugin.LookupServiceAs[plugin.UsageMeter](params.services.Lookup, plugin.ServiceCloudUsage); ok {
					fn(orgID, metric, n)
				}
			}
		},
		GetWorkspaceSetting: settings.GetWorkspaceSetting,
		GetGlobalSetting:    settings.GetGlobalSetting,
		SetGlobalSetting:    settings.SetGlobalSetting,
		SetWorkspaceSetting: settings.SetWorkspaceSetting,
		Enqueue: func(ctx context.Context, taskType string, payload []byte) error {
			if params.taskQueue != nil {
				return params.taskQueue.Enqueue(ctx, taskType, payload)
			}
			return errors.New("task queue not configured")
		},
		RegisterTask: func(taskType string, h func(ctx context.Context, payload []byte) error) {
			if params.taskQueue != nil {
				params.taskQueue.Register(taskType, h)
			}
		},
		PublishEvent:         events.PublishEvent,
		RegisterWebhookEvent: events.RegisterWebhookEvent,
		CacheGet:             cacheGet,
		CacheSet:             cacheSet,
		DeleteCache:          deleteCache,
		Cache:                scopedCache,
		GeoLookup: func(ip string) (string, string, string) {
			if geoResolver != nil {
				return geoResolver.Locate(ip)
			}
			return "", "", ""
		},
		ParseUA: func(ua string) (string, string, string) {
			info := geo.ParseUA(ua)
			return info.Device, info.Browser, info.OS
		},
		HandleRoot: func(h http.Handler) {
			if params.handleRoot != nil {
				params.handleRoot(h)
			}
		},
		HandleStatic: func(prefix string, fsys fs.FS) {
			if params.handleStatic != nil {
				params.handleStatic(prefix, fsys)
			}
		},
		Provide: servicesProvide,
		Lookup:  servicesLookup,
		Tracer: func(name string) trace.Tracer {
			return telemetry.Tracer(name)
		},
		Meter: func(name string) metric.Meter {
			return otel.GetMeterProvider().Meter(name)
		},
		StartSpan: func(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
			return telemetry.StartSpan(ctx, tracerName, spanName, opts...)
		},
		RegisterEndpoint: func(spec any) error {
			if params.endpointEngine != nil {
				return params.endpointEngine.Register(spec)
			}
			return errors.New("endpoint engine not configured")
		},
		RegisterTenantView: func(view plugin.TenantView) {
			_ = tenantsql.DefaultRegistry().Register(view)
		},
		RegisterReactor: eventbus.RegisterReactor,
		RegisterCron: func(name string, spec string, handler func(ctx context.Context) error) error {
			if params.cronEngine != nil {
				return params.cronEngine.Register(name, spec, handler)
			}
			return errors.New("cron engine not configured")
		},
		Cron: params.cronEngine,
	}

	if params.httpMode {
		pctx.PluginActive = func(orgID uint, p plugin.Plugin) bool {
			key := plugin.FeatureKey(p)
			if plugin.FeatureIsCore(pluginsList, key) {
				return true
			}
			if params.apiHandler != nil {
				return params.apiHandler.PluginEnabled(orgID, key)
			}
			return false
		}
		pctx.FeatureActive = func(orgID uint, featureKey string) bool {
			if plugin.FeatureIsCore(pluginsList, featureKey) {
				return true
			}
			// Unlike the route gate, this answers for content that is listed
			// before a workspace is chosen (help docs). PluginEnabled fails
			// closed at orgID 0 by design, so ask for the declared default
			// instead of inheriting a "disabled" that only means "no org yet".
			if orgID == 0 {
				if params.apiHandler != nil {
					return params.apiHandler.FeatureDefaultEnabled(featureKey)
				}
				return false
			}
			if params.apiHandler != nil {
				return params.apiHandler.PluginEnabled(orgID, featureKey)
			}
			return false
		}
		pctx.ActivePlugins = func() []plugin.Plugin {
			return pluginsList
		}
	}

	return pctx
}
