// Package api implements led's JSON HTTP API.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/jungley/led/config"
	"github.com/jungley/led/internal/auth"
	"github.com/jungley/led/internal/crypto"
	"github.com/jungley/led/internal/geo"
	"github.com/jungley/led/internal/mail"
	"gorm.io/gorm"
)

// Handler bundles dependencies shared by all API endpoints.
type Handler struct {
	cfg    *config.Config
	db     *gorm.DB
	cipher *crypto.Cipher
	auth   *auth.Manager
	sender mail.Sender
	geo    *geo.Resolver
}

func New(cfg *config.Config, db *gorm.DB, c *crypto.Cipher, a *auth.Manager, sender mail.Sender, g *geo.Resolver) *Handler {
	return &Handler{cfg: cfg, db: db, cipher: c, auth: a, sender: sender, geo: g}
}

// Routes returns the API mux mounted at /api/.
func (h *Handler) Routes() http.Handler {
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

	p("GET /api/links", h.listLinks)
	p("POST /api/links", h.createLink)
	p("GET /api/links/{id}", h.getLink)
	p("PUT /api/links/{id}", h.updateLink)
	p("DELETE /api/links/{id}", h.deleteLink)
	p("GET /api/links/{id}/stats", h.linkStats)
	p("GET /api/links/{id}/qr", h.linkQR)

	p("GET /api/dns/providers", h.dnsProviders)
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
	p("GET /api/emails/{id}", h.getEmail)
	p("PUT /api/emails/{id}", h.updateEmail)
	p("DELETE /api/emails/{id}", h.deleteEmail)
	p("POST /api/emails/send", h.sendEmail)

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
