package app

import (
	"io/fs"
	"net/http"
	"sync"

	"github.com/octarq-org/octarq/server/internal/api"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/cron"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/endpoint"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// PluginContextTestParams exports a parameter struct with exported fields for black-box testing.
type PluginContextTestParams struct {
	ApiHandler      *api.Handler
	Gdb             *gorm.DB
	Cipher          *crypto.Cipher
	Auth            *auth.Manager
	NotifRouter     *notification.Router
	Services        *plugin.Registry
	Geo             *geo.Resolver
	EndpointEngine  *endpoint.Engine
	TaskQueue       queue.Queue
	CronEngine      *cron.Engine
	SendMail        func(orgID uint, to, subject, htmlBody, textBody string) error
	LoginByEmail    func(w http.ResponseWriter, r *http.Request, email string) (uint, error)
	LoginByIdentity func(w http.ResponseWriter, r *http.Request, id plugin.ExternalIdentity) (uint, error)
	OnEmailMu       *sync.Mutex
	DeferredOnEmail *[]func(plugin.EmailEvent)
	HandleRoot      func(http.Handler)
	HandleStatic    func(prefix string, fsys fs.FS)
	HttpMode        bool
	Plugins         []plugin.Plugin
}

// BuildPluginContext exports buildPluginContext for black-box testing.
func (a *App) BuildPluginContext(p PluginContextTestParams) *plugin.Context {
	return a.buildPluginContext(pluginContextParams{
		apiHandler:      p.ApiHandler,
		gdb:             p.Gdb,
		cipher:          p.Cipher,
		auth:            p.Auth,
		notifRouter:     p.NotifRouter,
		services:        p.Services,
		geo:             p.Geo,
		endpointEngine:  p.EndpointEngine,
		taskQueue:       p.TaskQueue,
		cronEngine:      p.CronEngine,
		sendMail:        p.SendMail,
		loginByEmail:    p.LoginByEmail,
		loginByIdentity: p.LoginByIdentity,
		onEmailMu:       p.OnEmailMu,
		deferredOnEmail: p.DeferredOnEmail,
		handleRoot:      p.HandleRoot,
		handleStatic:    p.HandleStatic,
		httpMode:        p.HttpMode,
		plugins:         p.Plugins,
	})
}
