// Package api implements led's JSON HTTP API.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/auth"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/geo"
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
	plugins      []plugin.Plugin
}

func (h *Handler) SetPlugins(plugins []plugin.Plugin) {
	h.plugins = plugins
}


func New(cfg *config.Config, db *gorm.DB, c *crypto.Cipher, a *auth.Manager, g *geo.Resolver) *Handler {
	h := &Handler{
		cfg:          cfg,
		db:           db,
		cipher:       c,
		auth:         a,
		geo:          g,
		loginLimiter: newRateLimiter(5, 15*time.Minute), // 5 fails / 15 mins
		abuseLimiter: newRateLimiter(5, time.Hour),      // 5 reports / 1 hour
	}
	if cfg.BaseURL != "" {
		h.oauth = auth.NewOAuthHandler(db, cfg.BaseURL, a, c)
	}
	return h
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
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.me)

	// OAuth (no session required — these redirect to provider and back).
	if h.oauth != nil {
		mux.HandleFunc("GET /auth/begin/{provider}", h.oauth.Begin)
		mux.HandleFunc("GET /auth/callback/{provider}", h.oauth.Callback)
	}

	// Inbound email webhook (token-guarded, not session).
	mux.HandleFunc("POST /api/email/inbound", h.inbound)

	// Abuse reporting (public — no auth required to submit).
	mux.HandleFunc("POST /abuse", h.submitAbuse)

	// Everything below requires a session.
	p := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, h.auth.Require(fn))
	}

	p("GET /api/overview", h.overview)
	p("GET /api/settings", h.getSettings)
	p("PUT /api/settings", h.updateSettings)

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
	p("GET /api/org/members", h.listOrgMembers)
	p("POST /api/org/members", h.addOrgMember)
	p("DELETE /api/org/members/{userId}", h.removeOrgMember)

	// Menu and User Settings
	p("GET /api/menus", h.listMenus)
	p("GET /api/user/settings", h.getUserSettings)
	p("PUT /api/user/settings", h.updateUserSettings)

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
	return json.NewDecoder(r.Body).Decode(v)
}
