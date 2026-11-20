package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/origin"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type Handler struct {
	cfg                 *config.Config
	db                  *gorm.DB
	cipher              *crypto.Cipher
	auth                *auth.Manager
	geo                 *geo.Resolver
	oauth               *auth.OAuthHandler
	origins             *origin.Resolver
	loginLimiter        *rateLimiter
	loginAccountLimiter *rateLimiter
	recoveryLimiter     *rateLimiter
	registerLimiter     *rateLimiter
	abuseLimiter        *rateLimiter
	sendLimiter         *rateLimiter
	statusLimiter       *rateLimiter
	initErr             error
	plugins             []plugin.Plugin
	lookupTXT           func(name string) ([]string, error)
	lookupCNAME         func(name string) (string, error)
	queue               queue.Queue

	llmMu       sync.RWMutex
	llmResolver func(orgID uint) (llmprovider.Provider, error)

	lookupService  func(name string) (any, bool)
	endpointSource harness.EndpointSource
	host           plugin.Host
	humaAPI        huma.API

	hostOrgs           hostOrgCache
	storage            *plugin.LocalStorageService
	cronService        plugin.CronService
	vaultSvc           plugin.VaultService
	resendInviteLimits sync.Map
}

func (h *Handler) SetVaultService(vs plugin.VaultService) {
	h.vaultSvc = vs
	if h.oauth != nil {
		h.oauth.SetVaultService(vs)
	}
}

func (h *Handler) vault() plugin.VaultService {
	if h == nil {
		return nil
	}
	if h.vaultSvc != nil {
		return h.vaultSvc
	}
	if h.lookupService != nil {
		if vs, ok := plugin.LookupServiceAs[plugin.VaultService](h.lookupService, plugin.ServiceVault); ok {
			return vs
		}
	}
	return nil
}

func (h *Handler) SetHost(host plugin.Host) {
	h.host = host
}

func (h *Handler) workforce() plugin.Workforce {
	if h.host == nil {
		return plugin.EnsureHost(nil).Workforce()
	}
	return h.host.Workforce()
}

func (h *Handler) Storage() *plugin.LocalStorageService {
	return h.storage
}

func (h *Handler) SetStorage(s *plugin.LocalStorageService) {
	h.storage = s
}

func (h *Handler) SetPlugins(plugins []plugin.Plugin) {
	h.plugins = plugins
}

func (h *Handler) SetServiceLookup(lookup func(name string) (any, bool)) {
	h.lookupService = lookup
}

func (h *Handler) SetCronService(cs plugin.CronService) {
	h.cronService = cs
}

func (h *Handler) LookupService(name string) (any, bool) {
	if h.lookupService == nil {
		return nil, false
	}
	return h.lookupService(name)
}

func (h *Handler) Huma() huma.API {
	return h.humaAPI
}

func (h *Handler) InitErr() error {
	return h.initErr
}

func New(cfg *config.Config, db *gorm.DB, c *crypto.Cipher, a *auth.Manager, g *geo.Resolver, q queue.Queue) *Handler {
	trustProxy.Store(cfg.TrustProxy)
	h := &Handler{
		cfg:         cfg,
		db:          db,
		cipher:      c,
		auth:        a,
		geo:         g,
		queue:       q,
		lookupTXT:   net.LookupTXT,
		lookupCNAME: net.LookupCNAME,
		origins:     origin.NewResolver(db),
	}
	newLimiter := func(prefix string, limit int, window time.Duration) *rateLimiter {
		rl, err := newRateLimiter(cfg.RedisURL, prefix, limit, window)
		if err != nil {
			if h.initErr == nil {
				h.initErr = err
			}
			return nil
		}
		return rl
	}
	loginMax, loginAccountMax, loginWindow := cfg.LoginBudgets()
	h.loginLimiter = newLimiter("login", loginMax, loginWindow)
	h.loginAccountLimiter = newLimiter("login-acct", loginAccountMax, loginWindow)
	h.recoveryLimiter = newLimiter("recovery", 5, 15*time.Minute)
	h.registerLimiter = newLimiter("register", 5, time.Hour)
	h.abuseLimiter = newLimiter("abuse", 5, time.Hour)
	h.sendLimiter = newLimiter("send", 100, time.Hour)
	h.statusLimiter = newLimiter("status", 60, time.Minute)

	h.oauth = auth.NewOAuthHandler(db, a, nil)
	h.oauth.SetVaultLookup(h.vault)
	h.registerQueueHandlers(q)

	storageDir := cfg.StorageDir
	if storageDir == "" {
		storageDir = filepath.Join(os.TempDir(), "octarq-storage")
	}
	localStorage, err := plugin.NewLocalStorageService(storageDir, []byte(cfg.SecretKey), "")
	if err == nil {
		h.storage = localStorage
	}

	return h
}

func (h *Handler) registerQueueHandlers(q queue.Queue) {
	q.Register("abuse.notify", func(ctx context.Context, payload []byte) error {
		var rep models.AbuseReport
		if err := json.Unmarshal(payload, &rep); err != nil {
			return err
		}
		h.notifyAbuse(rep)
		return nil
	})
}

func (h *Handler) DataRetentionDays() int {
	v := h.getSetting(keyDataRetentionDays)
	if v == "" {
		return DefaultRetentionDays
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return DefaultRetentionDays
	}
	return n
}

func (h *Handler) origin(r *http.Request) string {
	return h.origins.Absolute(0, r, origin.Secure(r, trustProxy.Load()))
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	apierror.Write(w, r, status, code, msg)
}
