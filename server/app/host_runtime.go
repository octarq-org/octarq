package app

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
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
				if err := params.notifRouter.RegisterChannel(ch); err != nil {
					slog.Warn("failed to register notification channel", "err", err)
				}
			}
		},
		RegisterNotifier: func(typ string, send func(ctx context.Context, cfgJSON, text string) error) {
			if params.notifRouter != nil {
				if err := params.notifRouter.RegisterChannel(notification.NewLegacyNotifierAdapter(typ, "", send)); err != nil {
					slog.Warn("failed to register legacy notifier adapter", "type", typ, "err", err)
				}
			} else {
				notify.Register(typ, send)
			}
		},
		LoginByEmail:    loginByEmail,
		LoginByIdentity: loginByIdentity,
		BindIdentity:    bindIdentity,
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
		CacheGet:    cacheGet,
		CacheSet:    cacheSet,
		DeleteCache: deleteCache,
		Cache:       scopedCache,
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
			if err := tenantsql.DefaultRegistry().Register(view); err != nil {
				slog.Warn("failed to register tenant view", "view", view.Name, "err", err)
			}
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
