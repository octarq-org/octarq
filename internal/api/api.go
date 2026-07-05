// Package api implements led's JSON HTTP API.
package api

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/auth"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/geo"
	"github.com/Jungley8/led/internal/models"
	"github.com/Jungley8/led/internal/queue"
	"github.com/Jungley8/led/plugin"
	"gorm.io/gorm"
)

// Handler bundles dependencies shared by all API endpoints.
type Handler struct {
	cfg          *config.Config
	db           *gorm.DB
	cipher       *crypto.Cipher
	auth         *auth.Manager
	geo          *geo.Resolver
	oauth        *auth.OAuthHandler // nil if BaseURL not configured
	loginLimiter *rateLimiter
	abuseLimiter *rateLimiter
	sendLimiter  *rateLimiter // outbound-email rate cap, keyed by org
	plugins      []plugin.Plugin
	lookupTXT    func(name string) ([]string, error)
	lookupCNAME  func(name string) (string, error)
	queue        queue.Queue

	// emailHandlers are notified after each inbound email is stored. They are
	// registered by plugins via OnEmail and fired by emitEmail. Guarded by
	// emailMu because registration happens during plugin Mount (startup) while
	// dispatch happens on the inbound webhook path.
	emailMu       sync.RWMutex
	emailHandlers []func(plugin.EmailEvent)
}

func (h *Handler) SetPlugins(plugins []plugin.Plugin) {
	h.plugins = plugins
}

// OnEmail registers a handler invoked asynchronously after each inbound email is
// stored. It backs plugin.Context.OnEmail. Safe to call concurrently.
func (h *Handler) OnEmail(handler func(plugin.EmailEvent)) {
	if handler == nil {
		return
	}
	h.emailMu.Lock()
	h.emailHandlers = append(h.emailHandlers, handler)
	h.emailMu.Unlock()
}

// emitEmail dispatches an inbound-email event to every registered handler, each
// in its own goroutine so a slow handler (e.g. an LLM call) never blocks the
// webhook response or the other handlers.
func (h *Handler) emitEmail(e plugin.EmailEvent) {
	h.emailMu.RLock()
	handlers := h.emailHandlers
	h.emailMu.RUnlock()
	for _, fn := range handlers {
		go fn(e)
	}
}

func New(cfg *config.Config, db *gorm.DB, c *crypto.Cipher, a *auth.Manager, g *geo.Resolver, q queue.Queue) *Handler {
	trustProxy = cfg.TrustProxy
	h := &Handler{
		cfg:          cfg,
		db:           db,
		cipher:       c,
		auth:         a,
		geo:          g,
		queue:        q,
		loginLimiter: newRateLimiter(cfg.RedisURL, "login", 5, 15*time.Minute), // 5 fails / 15 mins
		abuseLimiter: newRateLimiter(cfg.RedisURL, "abuse", 5, time.Hour),      // 5 reports / 1 hour
		sendLimiter:  newRateLimiter(cfg.RedisURL, "send", 100, time.Hour),     // 100 outbound emails / org / hour
		lookupTXT:    net.LookupTXT,
		lookupCNAME:  net.LookupCNAME,
	}
	if cfg.BaseURL != "" {
		h.oauth = auth.NewOAuthHandler(db, cfg.BaseURL, a, c)
	}
	h.registerQueueHandlers(q)
	return h
}

func (h *Handler) registerQueueHandlers(q queue.Queue) {
	q.Register("link.crawl", func(ctx context.Context, payload []byte) error {
		var d struct {
			ID     uint   `json:"id"`
			Target string `json:"target"`
		}
		if err := json.Unmarshal(payload, &d); err != nil {
			return err
		}
		title, _ := fetchPageMeta(ctx, d.Target)
		if title != "" {
			return h.db.Model(&models.Link{}).Where("id = ?", d.ID).Update("title", title).Error
		}
		return nil
	})

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

// Routes returns the API mux mounted at /api/. It returns the concrete
// *http.ServeMux (not http.Handler) so plugins can mount additional /api/...
// routes onto the same mux before it is served.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Auth (no session required).
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.HandleFunc("POST /api/auth/2fa/verify", h.verify2FA)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.me)
	mux.HandleFunc("POST /api/auth/invite/accept", h.acceptInvite)
	mux.HandleFunc("GET /api/auth/config", h.authConfig)

	// OAuth (no session required — these redirect to provider and back).
	if h.oauth != nil {
		mux.HandleFunc("GET /auth/begin/{provider}", h.oauth.Begin)
		mux.HandleFunc("GET /auth/callback/{provider}", h.oauth.Callback)
	}

	// Inbound email webhook, n8n-style: the tenant slug and an unguessable per-org
	// token both live in the path, so the Cloudflare worker needs just this one URL
	// (no custom header). The slug scopes delivery to that org's mailboxes; the
	// token authenticates.
	mux.HandleFunc("POST /api/webhook/{orgSlug}/email/inbound/{token}", h.inbound)

	// Mail bounce/complaint webhook — same tenant-first, path-token scheme as
	// inbound: the slug names the org and the per-org token authenticates, so a
	// forged POST can't spam an org's notification channels.
	mux.HandleFunc("POST /api/webhook/{orgSlug}/email/bounce/{token}", h.emailBounceWebhook)

	// Abuse reporting (public — no auth required to submit).
	mux.HandleFunc("POST /abuse", h.submitAbuse)

	// Health check (public - no auth required).
	mux.HandleFunc("GET /api/health", h.health)

	// Everything below requires a session.
	p := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, h.auth.Require(fn))
	}

	p("GET /api/overview", h.overview)
	p("GET /api/settings", h.getSettings)
	p("PUT /api/settings", h.updateSettings)

	p("GET /api/webhooks", h.listWebhooks)
	p("POST /api/webhooks", h.createWebhook)
	p("PUT /api/webhooks/{id}", h.updateWebhook)
	p("DELETE /api/webhooks/{id}", h.deleteWebhook)

	p("GET /api/links", h.listLinks)
	p("GET /api/links/export.csv", h.exportLinksCSV)
	p("GET /api/links/metadata", h.linkMetadata)
	p("POST /api/links", h.createLink)
	p("GET /api/links/{id}", h.getLink)
	p("PUT /api/links/{id}", h.updateLink)
	p("DELETE /api/links/{id}", h.deleteLink)
	p("GET /api/links/{id}/stats", h.linkStats)
	p("GET /api/links/{id}/qr", h.linkQR)

	p("GET /api/dns/providers", h.dnsProviders)

	p("GET /api/provider-accounts", h.listProviderAccounts)
	p("POST /api/provider-accounts", h.createProviderAccount)
	p("PUT /api/provider-accounts/{id}", h.updateProviderAccount)
	p("DELETE /api/provider-accounts/{id}", h.deleteProviderAccount)

	p("GET /api/smtp-senders", h.listSMTPSenders)
	p("POST /api/smtp-senders", h.createSMTPSender)
	p("PUT /api/smtp-senders/{id}", h.updateSMTPSender)
	p("DELETE /api/smtp-senders/{id}", h.deleteSMTPSender)

	p("POST /api/domains/sync", h.syncDomains)
	p("GET /api/domains", h.listDomains)
	p("POST /api/domains", h.createDomain)
	p("PUT /api/domains/{id}", h.updateDomain)
	p("DELETE /api/domains/{id}", h.deleteDomain)
	p("GET /api/domains/{id}/verify-dns", h.verifyDomainDNS)
	p("GET /api/domains/{id}/records", h.listRecords)
	p("POST /api/domains/{id}/records", h.createRecord)
	p("PUT /api/domains/{id}/records/{rid}", h.updateRecord)
	p("DELETE /api/domains/{id}/records/{rid}", h.deleteRecord)

	p("GET /api/mailboxes", h.listMailboxes)
	p("POST /api/mailboxes", h.createMailbox)
	p("PUT /api/mailboxes/{id}", h.updateMailbox)
	p("DELETE /api/mailboxes/{id}", h.deleteMailbox)
	p("GET /api/emails", h.listEmails)
	p("POST /api/emails/read-all", h.readAllEmails)
	p("GET /api/emails/{id}", h.getEmail)
	p("GET /api/emails/{id}/raw", h.rawEmail)
	p("PUT /api/emails/{id}", h.updateEmail)
	p("DELETE /api/emails/{id}", h.deleteEmail)
	p("POST /api/emails/send", h.sendEmail)

	p("GET /api/tokens", h.listTokens)
	p("POST /api/tokens", h.createToken)
	p("DELETE /api/tokens/{id}", h.deleteToken)

	p("GET /api/notification-channels", h.listNotificationChannels)
	p("POST /api/notification-channels", h.createNotificationChannel)
	p("PUT /api/notification-channels/{id}", h.updateNotificationChannel)
	p("DELETE /api/notification-channels/{id}", h.deleteNotificationChannel)
	p("POST /api/notification-channels/{id}/test", h.testNotificationChannel)

	// Abuse reports: submit is public, list/update require admin session.
	p("GET /api/abuse", h.listAbuseReports)
	p("PUT /api/abuse/{id}", h.updateAbuseReport)

	// Audit log (session required, read-only for now).
	p("GET /api/audit", h.listAuditLogs)

	// Multi-tenant Organization Management
	p("POST /api/auth/switch-org", h.switchOrg)
	p("GET /api/orgs", h.listOrgs)
	p("POST /api/orgs", h.createOrg)
	p("PUT /api/org", h.updateOrg)
	p("GET /api/org/members", h.listOrgMembers)
	p("POST /api/org/members", h.addOrgMember)
	p("DELETE /api/org/members/{userId}", h.removeOrgMember)

	// Data portability (GDPR/CCPA): export everything, or destroy it.
	p("GET /api/account/export", h.exportAccount)
	p("DELETE /api/account/data", h.purgeAccount)

	// Operator account security: session revocation + TOTP 2FA.
	p("POST /api/auth/logout-all", h.logoutAll)
	p("GET /api/auth/sessions", h.listSessions)
	p("DELETE /api/auth/sessions/{id}", h.revokeSession)
	p("GET /api/auth/2fa/status", h.twoFAStatus)
	p("POST /api/auth/2fa/setup", h.setup2FA)
	p("POST /api/auth/2fa/enable", h.enable2FA)
	p("POST /api/auth/2fa/disable", h.disable2FA)

	// Menu and User Settings
	p("GET /api/menus", h.listMenus)
	p("GET /api/plugins", h.listPlugins)
	p("PUT /api/plugins/{name}", h.updatePlugin)
	p("GET /api/user/settings", h.getUserSettings)
	p("PUT /api/user/settings", h.updateUserSettings)

	// URL rewriting for API versioning (/api/v1/xxx -> /api/xxx)
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/" + strings.TrimPrefix(r.URL.Path, "/api/v1/")
		mux.ServeHTTP(w, r)
	})

	return mux
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v) // 1MB limit
}

func idParam(r *http.Request) (uint, bool) {
	v, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(v), true
}
