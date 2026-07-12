// Package api implements octarq's JSON HTTP API.
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

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/queue"
	"github.com/octarq-org/octarq/llmprovider"
	"github.com/octarq-org/octarq/plugin"
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

	// llmResolver supplies the LLM backend for the single-step AI assists
	// (ai.go). Defaults to the env-backed envLLMResolver; the Pro ai plugin
	// swaps in its DB-backed provider via SetLLMResolver during Mount.
	llmMu       sync.RWMutex
	llmResolver func() (llmprovider.Provider, error)

	// emailHandlers are notified after each inbound email is stored. They are
	// registered by plugins via OnEmail and fired by emitEmail. Guarded by
	// emailMu because registration happens during plugin Mount (startup) while
	// dispatch happens on the inbound webhook path.
	emailMu       sync.RWMutex
	emailHandlers []func(plugin.EmailEvent)
	humaAPI       huma.API
}

func (h *Handler) SetPlugins(plugins []plugin.Plugin) {
	h.plugins = plugins
}

func (h *Handler) Huma() huma.API {
	return h.humaAPI
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
		llmResolver:  envLLMResolver(),
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

	config := huma.DefaultConfig("Octarq API", "1.0.0")
	api := humago.New(mux, config)
	h.humaAPI = api

	// Override validation error status from 422 to 400 for consistency with tests and clients.
	oldNewError := huma.NewError
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		if status == 422 {
			status = 400
		}
		return oldNewError(status, msg, errs...)
	}

	// Early authentication middleware to avoid validation failures returning 400/422 for unauthenticated requests.
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		path := ctx.URL().Path
		if strings.HasPrefix(path, "/api/") &&
			!strings.HasPrefix(path, "/api/auth/login") &&
			!strings.HasPrefix(path, "/api/auth/register") &&
			!strings.HasPrefix(path, "/api/auth/2fa/verify") &&
			!strings.HasPrefix(path, "/api/auth/logout") &&
			!strings.HasPrefix(path, "/api/auth/config") &&
			!strings.HasPrefix(path, "/api/auth/invite/accept") &&
			!strings.HasPrefix(path, "/api/webhook/") &&
			!strings.HasPrefix(path, "/api/health") {

			r, _ := humago.Unwrap(ctx)
			if r != nil {
				r2, ok := h.auth.AuthenticateRequest(r)
				if !ok {
					huma.WriteErr(api, ctx, http.StatusUnauthorized, "unauthorized")
					return
				}
				ctx = huma.WithContext(ctx, r2.Context())
			}
		}
		next(ctx)
	})

	// Auth (no session required).
	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      "POST",
		Path:        "/api/auth/login",
		Summary:     "Log in",
		Tags:        []string{"Auth"},
	}, h.loginHuma)

	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/register", Summary: "Register", Tags: []string{"Auth"}}, h.register)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/2fa/verify", Summary: "Verify 2FA", Tags: []string{"Auth"}}, h.verify2FA)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/logout", Summary: "Log out", Tags: []string{"Auth"}}, h.logout)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/me", Summary: "Me", Tags: []string{"Auth"}}, h.me)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/invite/accept", Summary: "Accept Invite", Tags: []string{"Auth"}}, h.acceptInvite)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/config", Summary: "Auth Config", Tags: []string{"Auth"}}, h.authConfig)

	// OAuth (no session required — these redirect to provider and back).
	if h.oauth != nil {
		mux.HandleFunc("GET /auth/begin/{provider}", h.oauth.Begin)
		mux.HandleFunc("GET /auth/callback/{provider}", h.oauth.Callback)
	}

	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/webhook/{orgSlug}/email/inbound/{token}", Summary: "Inbound Email", Tags: []string{"Webhook"}}, h.inbound)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/webhook/{orgSlug}/email/bounce/{token}", Summary: "Email Bounce Webhook", Tags: []string{"Webhook"}}, h.emailBounceWebhook)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/abuse", Summary: "Submit Abuse", Tags: []string{"Public"}, DefaultStatus: 201}, h.submitAbuse)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/health", Summary: "Health Check", Tags: []string{"Public"}}, h.health)

	// MCP SSE and Streamable HTTP endpoints.
	mux.Handle("/api/mcp/sse", h.mcpSSEHandler())
	mux.Handle("/api/mcp/stream", h.mcpStreamHandler())

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/overview", Summary: "Overview Stats", Tags: []string{"Dashboard"}}, h.overview)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/settings", Summary: "Get Settings", Tags: []string{"Settings"}}, h.getSettings)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/settings", Summary: "Update Settings", Tags: []string{"Settings"}}, h.updateSettings)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/instance-settings", Summary: "Get Instance Settings", Tags: []string{"Settings"}}, h.getInstanceSettings)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/instance-settings", Summary: "Update Instance Settings", Tags: []string{"Settings"}}, h.updateInstanceSettings)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/webhooks", Summary: "List Webhooks", Tags: []string{"Webhooks"}}, h.listWebhooks)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/webhooks", Summary: "Create Webhook", Tags: []string{"Webhooks"}, DefaultStatus: 201}, h.createWebhook)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/webhooks/{id}", Summary: "Update Webhook", Tags: []string{"Webhooks"}}, h.updateWebhook)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/webhooks/{id}", Summary: "Delete Webhook", Tags: []string{"Webhooks"}}, h.deleteWebhook)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links", Summary: "List Links", Tags: []string{"Links"}}, h.listLinks)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/export.csv", Summary: "Export Links CSV", Tags: []string{"Links"}}, h.exportLinksCSV)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/metadata", Summary: "Link Metadata", Tags: []string{"Links"}}, h.linkMetadata)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/links", Summary: "Create Link", Tags: []string{"Links"}, DefaultStatus: 201}, h.createLink)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}", Summary: "Get Link", Tags: []string{"Links"}}, h.getLink)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/links/{id}", Summary: "Update Link", Tags: []string{"Links"}}, h.updateLink)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/links/{id}", Summary: "Delete Link", Tags: []string{"Links"}}, h.deleteLink)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/stats", Summary: "Link Stats", Tags: []string{"Links"}}, h.linkStats)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/qr", Summary: "Link QR Code", Tags: []string{"Links"}}, h.linkQR)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/dns/providers", Summary: "DNS Providers", Tags: []string{"DNS"}}, h.dnsProviders)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/provider-accounts", Summary: "List Provider Accounts", Tags: []string{"Providers"}}, h.listProviderAccounts)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/provider-accounts", Summary: "Create Provider Account", Tags: []string{"Providers"}, DefaultStatus: 201}, h.createProviderAccount)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/provider-accounts/{id}", Summary: "Update Provider Account", Tags: []string{"Providers"}}, h.updateProviderAccount)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/provider-accounts/{id}", Summary: "Delete Provider Account", Tags: []string{"Providers"}}, h.deleteProviderAccount)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/smtp-senders", Summary: "List SMTP Senders", Tags: []string{"SMTP"}}, h.listSMTPSenders)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/smtp-senders", Summary: "Create SMTP Sender", Tags: []string{"SMTP"}, DefaultStatus: 201}, h.createSMTPSender)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/smtp-senders/{id}", Summary: "Update SMTP Sender", Tags: []string{"SMTP"}}, h.updateSMTPSender)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/smtp-senders/{id}", Summary: "Delete SMTP Sender", Tags: []string{"SMTP"}}, h.deleteSMTPSender)

	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/domains/sync", Summary: "Sync Domains", Tags: []string{"Domains"}}, h.syncDomains)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/domains", Summary: "List Domains", Tags: []string{"Domains"}}, h.listDomains)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/domains", Summary: "Create Domain", Tags: []string{"Domains"}, DefaultStatus: 201}, h.createDomain)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/domains/{id}", Summary: "Update Domain", Tags: []string{"Domains"}}, h.updateDomain)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/domains/{id}", Summary: "Delete Domain", Tags: []string{"Domains"}}, h.deleteDomain)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/domains/{id}/verify-dns", Summary: "Verify Domain DNS", Tags: []string{"Domains"}}, h.verifyDomainDNS)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/domains/{id}/records", Summary: "List DNS Records", Tags: []string{"Domains"}}, h.listRecords)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/domains/{id}/records", Summary: "Create DNS Record", Tags: []string{"Domains"}, DefaultStatus: 201}, h.createRecord)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/domains/{id}/records/{rid}", Summary: "Update DNS Record", Tags: []string{"Domains"}}, h.updateRecord)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/domains/{id}/records/{rid}", Summary: "Delete DNS Record", Tags: []string{"Domains"}}, h.deleteRecord)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/mailboxes", Summary: "List Mailboxes", Tags: []string{"Mailboxes"}}, h.listMailboxes)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/mailboxes", Summary: "Create Mailbox", Tags: []string{"Mailboxes"}, DefaultStatus: 201}, h.createMailbox)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/mailboxes/{id}", Summary: "Update Mailbox", Tags: []string{"Mailboxes"}}, h.updateMailbox)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/mailboxes/{id}", Summary: "Delete Mailbox", Tags: []string{"Mailboxes"}}, h.deleteMailbox)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails", Summary: "List Emails", Tags: []string{"Emails"}}, h.listEmails)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/emails/read-all", Summary: "Mark All Emails Read", Tags: []string{"Emails"}}, h.readAllEmails)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails/{id}", Summary: "Get Email", Tags: []string{"Emails"}}, h.getEmail)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails/{id}/raw", Summary: "Get Raw Email EML", Tags: []string{"Emails"}}, h.rawEmail)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/emails/{id}", Summary: "Update Email State", Tags: []string{"Emails"}}, h.updateEmail)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/emails/{id}", Summary: "Delete Email", Tags: []string{"Emails"}}, h.deleteEmail)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/emails/send", Summary: "Send Email", Tags: []string{"Emails"}}, h.sendEmail)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/ai/assist/status", Summary: "Get AI Assist Status", Tags: []string{"AI"}}, h.aiStatus)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/ai/assist/suggest-slug", Summary: "Suggest Link Slug via AI", Tags: []string{"AI"}}, h.aiSuggestSlug)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/ai/assist/summarize-email/{id}", Summary: "Summarize Email via AI", Tags: []string{"AI"}}, h.aiSummarizeEmail)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/tokens", Summary: "List API Tokens", Tags: []string{"Tokens"}}, h.listTokens)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/tokens", Summary: "Create API Token", Tags: []string{"Tokens"}, DefaultStatus: 201}, h.createToken)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/tokens/{id}", Summary: "Delete API Token", Tags: []string{"Tokens"}}, h.deleteToken)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/notification-channels", Summary: "List Notification Channels", Tags: []string{"Notification Channels"}}, h.listNotificationChannels)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/notification-channels", Summary: "Create Notification Channel", Tags: []string{"Notification Channels"}, DefaultStatus: 201}, h.createNotificationChannel)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/notification-channels/{id}", Summary: "Update Notification Channel", Tags: []string{"Notification Channels"}}, h.updateNotificationChannel)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/notification-channels/{id}", Summary: "Delete Notification Channel", Tags: []string{"Notification Channels"}}, h.deleteNotificationChannel)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/notification-channels/{id}/test", Summary: "Test Notification Channel", Tags: []string{"Notification Channels"}}, h.testNotificationChannel)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/abuse", Summary: "List Abuse Reports", Tags: []string{"Abuse"}}, h.listAbuseReports)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/abuse/{id}", Summary: "Update Abuse Report", Tags: []string{"Abuse"}}, h.updateAbuseReport)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/audit", Summary: "List Audit Logs", Tags: []string{"Audit Logs"}}, h.listAuditLogs)

	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/switch-org", Summary: "Switch Org", Tags: []string{"Org Management"}}, h.switchOrg)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/orgs", Summary: "List Orgs", Tags: []string{"Org Management"}}, h.listOrgs)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/orgs", Summary: "Create Org", Tags: []string{"Org Management"}, DefaultStatus: 201}, h.createOrg)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/org", Summary: "Update Org Details", Tags: []string{"Org Management"}}, h.updateOrg)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/org/members", Summary: "List Org Members", Tags: []string{"Org Management"}}, h.listOrgMembers)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/org/members", Summary: "Add Org Member", Tags: []string{"Org Management"}}, h.addOrgMember)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/org/members/{userId}", Summary: "Remove Org Member", Tags: []string{"Org Management"}}, h.removeOrgMember)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/account/export", Summary: "Export Org Data", Tags: []string{"Account"}}, h.exportAccount)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/account/data", Summary: "Purge Org Data", Tags: []string{"Account"}}, h.purgeAccount)

	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/logout-all", Summary: "Logout All Sessions", Tags: []string{"Sessions"}}, h.logoutAll)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/sessions", Summary: "List Active Sessions", Tags: []string{"Sessions"}}, h.listSessions)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/auth/sessions/{id}", Summary: "Revoke Session", Tags: []string{"Sessions"}}, h.revokeSession)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/2fa/status", Summary: "Get 2FA Status", Tags: []string{"2FA"}}, h.twoFAStatus)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/2fa/setup", Summary: "Setup 2FA", Tags: []string{"2FA"}}, h.setup2FA)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/2fa/enable", Summary: "Enable 2FA", Tags: []string{"2FA"}}, h.enable2FA)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/2fa/disable", Summary: "Disable 2FA", Tags: []string{"2FA"}}, h.disable2FA)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/menus", Summary: "List Menu Toggles", Tags: []string{"UI Settings"}}, h.listMenus)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/plugins", Summary: "List Plugins", Tags: []string{"UI Settings"}}, h.listPlugins)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/plugins/{name}", Summary: "Toggle Plugin", Tags: []string{"UI Settings"}}, h.updatePlugin)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/user/settings", Summary: "Get User Settings", Tags: []string{"UI Settings"}}, h.getUserSettings)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/user/settings", Summary: "Update User Settings", Tags: []string{"UI Settings"}}, h.updateUserSettings)

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
