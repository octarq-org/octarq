// Package api implements octarq's JSON HTTP API.
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
	"github.com/octarq-org/octarq/server/internal/agentstore"
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

// Handler bundles dependencies shared by all API endpoints.
type Handler struct {
	cfg    *config.Config
	db     *gorm.DB
	cipher *crypto.Cipher
	auth   *auth.Manager
	geo    *geo.Resolver
	oauth  *auth.OAuthHandler
	// origins derives the absolute origin outbound links are built from, from
	// the request that asks for them. See origin and h.origin below.
	origins      *origin.Resolver
	loginLimiter *rateLimiter
	// recoveryLimiter bounds the unauthenticated password-reset / verification-resend
	// endpoints. It is deliberately SEPARATE from loginLimiter: those endpoints must
	// count every request (there is no "failed" reset to count), and sharing the
	// login budget would let anyone lock a whole NAT out of logging in by firing a
	// handful of reset requests.
	recoveryLimiter *rateLimiter
	// registerLimiter bounds public sign-ups per IP (5/hour). It is deliberately
	// separate from loginLimiter too: a successful registration used to reset the
	// shared login budget and never counted against it, so sign-up was
	// effectively unlimited from a single IP.
	registerLimiter *rateLimiter
	abuseLimiter    *rateLimiter
	sendLimiter     *rateLimiter // outbound-email rate cap, keyed by org
	statusLimiter   *rateLimiter
	plugins         []plugin.Plugin
	lookupTXT       func(name string) ([]string, error)
	lookupCNAME     func(name string) (string, error)
	queue           queue.Queue

	// llmResolver supplies the LLM backend for the single-step AI assists
	// (ai.go). Defaults to the env-backed envLLMResolver; the Pro ai plugin
	// swaps in its DB-backed provider via SetLLMResolverForOrg during Mount.
	llmMu       sync.RWMutex
	llmResolver func(orgID uint) (llmprovider.Provider, error)

	// lookupService resolves named services for plugins (see
	// plugin.LookupServiceAs), and is what export/purge/overview and the AI
	// assists reach through when a plugin is absent from this build. Registered
	// by the app via SetServiceLookup.
	lookupService  func(name string) (any, bool)
	endpointSource harness.EndpointSource
	// harnessSpecProvider resolves the published agent harness spec of a
	// workspace. Nil means the built-in default applies.
	harnessSpecProvider func(ctx context.Context, orgID uint) (harness.HarnessSpec, error)
	// harnessStore persists the published harness versions of a workspace.
	harnessStore *agentstore.HarnessStore
	humaAPI      huma.API

	// hostOrgs caches Host→org resolution for per-workspace branding on the
	// public, pre-auth config endpoint. See host_org.go.
	hostOrgs           hostOrgCache
	storage            *plugin.LocalStorageService
	cronService        plugin.CronService
	resendInviteLimits sync.Map
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

func New(cfg *config.Config, db *gorm.DB, c *crypto.Cipher, a *auth.Manager, g *geo.Resolver, q queue.Queue) *Handler {
	trustProxy.Store(cfg.TrustProxy)
	h := &Handler{
		cfg:             cfg,
		db:              db,
		cipher:          c,
		auth:            a,
		geo:             g,
		queue:           q,
		loginLimiter:    newRateLimiter(cfg.RedisURL, "login", 5, 15*time.Minute),    // 5 fails / 15 mins
		recoveryLimiter: newRateLimiter(cfg.RedisURL, "recovery", 5, 15*time.Minute), // 5 reset/verify requests / 15 mins
		registerLimiter: newRateLimiter(cfg.RedisURL, "register", 5, time.Hour),      // 5 sign-ups / hour
		abuseLimiter:    newRateLimiter(cfg.RedisURL, "abuse", 5, time.Hour),         // 5 reports / 1 hour
		sendLimiter:     newRateLimiter(cfg.RedisURL, "send", 100, time.Hour),        // 100 outbound emails / org / hour
		statusLimiter:   newRateLimiter(cfg.RedisURL, "status", 60, time.Minute),     // 60 requests / minute
		lookupTXT:       net.LookupTXT,
		lookupCNAME:     net.LookupCNAME,
		llmResolver:     envLLMResolver(),
		origins:         origin.NewResolver(db),
	}
	h.oauth = auth.NewOAuthHandler(db, a, c)
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

// DataRetentionDays returns the configured retention period for click events.
// Returns 0 if retention is disabled, DefaultRetentionDays if unset.
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

// --- helpers ---

// origin returns the absolute origin ("https://app.example.com") that links
// mailed out of this request must be rooted at, or "" when the request arrived
// on a hostname this instance has not registered.
//
// "" is not a failure to handle — it degrades the link to a relative path, the
// same thing an unset OCTARQ_BASE_URL used to produce. That is deliberate: a
// link that cannot be opened from a mail client is a support ticket, whereas a
// link built from a forged Host header is an account takeover (CWE-640).
//
// orgID is 0 (any hostname this instance owns) because these flows are
// instance-wide: password reset starts before anyone is authenticated, so the
// recipient's workspace is not known when the URL is built.
func (h *Handler) origin(r *http.Request) string {
	return h.origins.Absolute(0, r, origin.Secure(r, trustProxy.Load()))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeErr emits the one error envelope from a plain net/http handler — the
// paths that run outside huma (the CSRF guard, the MCP transports). Callers
// that have a more specific discriminator than the status pass it as code;
// passing "" takes the default for the status.
func writeErr(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	apierror.Write(w, r, status, code, msg)
}
