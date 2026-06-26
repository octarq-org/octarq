// Package api implements led's JSON HTTP API.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/jungley/led/config"
	"github.com/jungley/led/internal/auth"
	"github.com/jungley/led/internal/crypto"
	"github.com/jungley/led/internal/geo"
	"gorm.io/gorm"
)

// Handler bundles dependencies shared by all API endpoints.
type Handler struct {
	cfg      *config.Config
	db       *gorm.DB
	cipher   *crypto.Cipher
	auth     *auth.Manager
	geo      *geo.Resolver
}

func New(cfg *config.Config, db *gorm.DB, c *crypto.Cipher, a *auth.Manager, g *geo.Resolver) *Handler {
	return &Handler{cfg: cfg, db: db, cipher: c, auth: a, geo: g}
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

	// Inbound email webhook (token-guarded, not session).
	mux.HandleFunc("POST /api/email/inbound", h.inbound)

	// Everything below requires a session.
	p := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, h.auth.Require(fn))
	}

	p("GET /api/overview", h.overview)
	p("GET /api/settings", h.getSettings)
	p("PUT /api/settings", h.updateSettings)

	p("GET /api/links", h.listLinks)
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

	p("GET /api/ssh-keys", h.listSSHKeys)
	p("POST /api/ssh-keys", h.createSSHKey)
	p("DELETE /api/ssh-keys/{id}", h.deleteSSHKey)

	p("GET /api/vps", h.listVPS)
	p("POST /api/vps", h.createVPS)
	p("PUT /api/vps/{id}", h.updateVPS)
	p("DELETE /api/vps/{id}", h.deleteVPS)
	p("GET /api/vps/{id}/terminal", h.vpsTerminal)

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
