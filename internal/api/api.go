// Package api implements octarq's JSON HTTP API.
package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/apierror"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/queue"
	"github.com/octarq-org/octarq/llmprovider"
	"github.com/octarq-org/octarq/origin"
	"github.com/octarq-org/octarq/plugin"
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
	lookupService func(name string) (any, bool)
	humaAPI       huma.API

	// hostOrgs caches Host→org resolution for per-workspace branding on the
	// public, pre-auth config endpoint. See host_org.go.
	hostOrgs hostOrgCache
}

func (h *Handler) SetPlugins(plugins []plugin.Plugin) {
	h.plugins = plugins
}

func (h *Handler) SetServiceLookup(lookup func(name string) (any, bool)) {
	h.lookupService = lookup
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
	trustProxy = cfg.TrustProxy
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

// Routes returns the API mux mounted at /api/. It returns the concrete
// *http.ServeMux (not http.Handler) so plugins can mount additional /api/...
// routes onto the same mux before it is served.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	config := huma.DefaultConfig("Octarq API", "1.0.0")
	config.Info.Description = apiDescription
	config.Servers = []*huma.Server{{URL: "/", Description: "This instance"}}
	// Hooks run on EVERY operation added to the document, including the ones
	// plugins register after Routes() returns. That is the point: a rule
	// enforced only over the core's own registrations is a rule half the
	// surface escapes.
	config.OnAddOperation = append(config.OnAddOperation, normalizeValidationResponse, applyDeprecation)

	api := humago.New(mux, config)
	h.humaAPI = api

	// The single error envelope is installed process-wide in errors.go's init().
	// It used to be assigned here, wrapping whatever was already installed —
	// which nested on the second Routes() call and silently rewrote status codes
	// other repositories had declared. See errors.go.

	// Deprecation signalling runs first so the headers are on the response even
	// when a later middleware refuses the request: a caller whose integration
	// has a deadline should learn about it from a 401 too.
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		deprecationHeaders(ctx)
		next(ctx)
	})

	// Early authentication middleware to avoid validation failures returning 400/422 for unauthenticated requests.
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		// A plugin may mark a SPECIFIC operation as public by setting
		// Metadata["public"] = true on its huma.Operation. Such a handler
		// authenticates its own callers (buyer-session cookie, or is
		// intentionally public), so the dashboard-auth gate is skipped for it —
		// and ONLY for it. This is exact per-operation opt-in: it can never
		// widen to sibling routes the way a path-prefix allowlist would.
		// OPERATOR routes must NEVER be marked public.
		if op := ctx.Operation(); op != nil {
			if v, _ := op.Metadata["public"].(bool); v {
				next(ctx)
				return
			}
		}
		// The prefix allowlist lives in public_endpoints.go next to the
		// inventory that reports on it. Keeping the gate and its inventory on
		// one list is what stops them drifting apart — a prefix added here but
		// not there would exempt a route the registry test never sees.
		if !isPublicPath(ctx.URL().Path) {
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

	// Auth routes.
	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      "POST",
		Path:        "/api/auth/login",
		Summary:     "Log in",
		Description: "Exchange credentials for a browser session. On success the `octarq_session` cookie is set " +
			"along with the CSRF token cookie; subsequent cookie-authenticated writes must echo that token in " +
			"`X-CSRF-Token`. When the account has 2FA enabled the response asks for a second factor instead of " +
			"completing the login — finish it at `POST /api/auth/2fa/verify`.\n\n" +
			"Machine clients should not use this endpoint: create an API token (see the Tokens tag) and send " +
			"`Authorization: Bearer <token>` instead. Failed attempts are rate limited per IP (5 per 15 minutes) " +
			"and answer 429.",
		Tags:   []string{"Auth"},
		Errors: []int{401, 429},
	}, h.loginHuma)

	huma.Register(api, huma.Operation{
		OperationID: "register",
		Method:      "POST",
		Path:        "/api/auth/register",
		Summary:     "Register",
		Description: "Create an account and its first workspace. Refused with 403 when the instance has public " +
			"sign-up disabled, and rate limited to 5 sign-ups per IP per hour. When email verification is " +
			"required the account exists but cannot sign in until the emailed link is followed.",
		Tags:   []string{"Auth"},
		Errors: []int{403, 409, 429},
	}, h.register)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/2fa/verify", Summary: "Verify 2FA", Tags: []string{"Auth"}}, h.verify2FA)
	huma.Register(api, huma.Operation{
		OperationID: "logout",
		Method:      "POST",
		Path:        "/api/auth/logout",
		Summary:     "Log out",
		Description: "Ends the current session and clears its cookies. Idempotent: logging out without a valid " +
			"session still succeeds, so a client recovering from an expired session need not special-case it. " +
			"Other sessions of the same user are untouched — use `POST /api/auth/logout-all` for those.",
		Tags: []string{"Auth"},
	}, h.logout)
	huma.Register(api, huma.Operation{
		OperationID: "getCurrentUser",
		Method:      "GET",
		Path:        "/api/auth/me",
		Summary:     "Get the authenticated user",
		Description: "Returns the caller's identity, their role in the active workspace, and which workspace is " +
			"active. This is the canonical way to test whether a session or bearer token is still valid: a 401 " +
			"here means re-authenticate. The active workspace is a property of the session, not of the user — " +
			"change it with `POST /api/auth/switch-org`.",
		Tags:   []string{"Auth"},
		Errors: []int{401},
	}, h.me)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/invite/accept", Summary: "Accept Invite", Tags: []string{"Auth"}}, h.acceptInvite)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/password", Summary: "Change Password", Tags: []string{"Auth"}}, h.changePassword)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/auth/email", Summary: "Change Email", Tags: []string{"Auth"}}, h.changeEmail)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/config", Summary: "Auth Config", Tags: []string{"Auth"}}, h.authConfig)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/methods", Summary: "Auth Methods", Tags: []string{"Auth"}, Metadata: map[string]any{"public": true}}, h.getAuthMethods)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/forgot", Summary: "Forgot Password", Tags: []string{"Auth"}, Metadata: map[string]any{"public": true}}, h.forgotPassword)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/reset", Summary: "Reset Password", Tags: []string{"Auth"}, Metadata: map[string]any{"public": true}}, h.resetPassword)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/verify-email", Summary: "Verify Email", Tags: []string{"Auth"}, Metadata: map[string]any{"public": true}}, h.verifyEmail)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/resend-verification", Summary: "Resend Verification Email", Tags: []string{"Auth"}, Metadata: map[string]any{"public": true}}, h.resendVerification)

	// OAuth (no session required — these redirect to provider and back). The
	// routes always exist; the handler answers 503 when no provider is
	// configured, or when the request arrived on a hostname it cannot build a
	// callback URL for.
	mux.HandleFunc("GET /auth/begin/{provider}", h.oauth.Begin)
	mux.HandleFunc("GET /auth/callback/{provider}", h.oauth.Callback)

	huma.Register(api, huma.Operation{Method: "POST", Path: "/abuse", Summary: "Submit Abuse", Tags: []string{"Public"}, DefaultStatus: 201}, h.submitAbuse)
	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      "GET",
		Path:        "/api/health",
		Summary:     "Health check",
		Description: "Liveness probe: answers 200 whenever the process is serving. It deliberately does NOT check " +
			"the database or any dependency, so an orchestrator does not restart a healthy process over a " +
			"transient outage downstream. For dependency state use `GET /api/status`.",
		Tags: []string{"Public"},
	}, h.health)
	huma.Register(api, huma.Operation{
		OperationID: "subsystemStatus",
		Method:      "GET",
		Path:        "/api/status",
		Summary:     "Subsystem status",
		Description: "Per-subsystem health (database, queue, mail, DNS) behind the public status page. Unlike " +
			"`/api/health` this does reach dependencies, so it is rate limited to 60 requests per minute per IP.",
		Tags:     []string{"Public"},
		Errors:   []int{429},
		Metadata: map[string]any{"public": true},
	}, h.subsystemStatus)

	// MCP SSE and Streamable HTTP endpoints.
	mux.Handle("/api/mcp/sse", h.mcpSSEHandler())
	mux.Handle("/api/mcp/stream", h.mcpStreamHandler())

	huma.Register(api, huma.Operation{
		OperationID: "overview",
		Method:      "GET",
		Path:        "/api/overview",
		Summary:     "Workspace overview",
		Description: "Aggregate counters and recent activity for the active workspace — what the dashboard home " +
			"renders. Scoped to the workspace on the session; it never aggregates across workspaces, even for " +
			"an instance administrator.",
		Tags:   []string{"Dashboard"},
		Errors: []int{401},
	}, h.overview)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/settings", Summary: "Get Settings", Tags: []string{"Settings"}}, h.getSettings)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/settings/inbound-token", Summary: "Get Inbound Webhook Token", Tags: []string{"Settings"}}, h.getInboundToken)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/settings", Summary: "Update Settings", Tags: []string{"Settings"}}, h.updateSettings)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/instance-settings", Summary: "Get Instance Settings", Tags: []string{"Settings"}}, h.getInstanceSettings)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/instance-settings", Summary: "Update Instance Settings", Tags: []string{"Settings"}}, h.updateInstanceSettings)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/instance/plugins", Summary: "List Instance Plugins", Tags: []string{"Settings"}}, h.listInstancePlugins)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/instance/menus", Summary: "List Instance Menus", Tags: []string{"Settings"}}, h.listInstanceMenus)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/instance/build", Summary: "Get Instance Build Info", Tags: []string{"Settings"}}, h.instanceBuild)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/instance/readiness", Summary: "Get Instance Readiness", Tags: []string{"Settings"}}, h.instanceReadiness)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/admin/backup", Summary: "Download Database Backup", Tags: []string{"Settings"}}, h.downloadBackup)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/webhooks", Summary: "List Webhooks", Tags: []string{"Webhooks"}}, h.listWebhooks)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/webhooks/events", Summary: "List Webhook Event Types", Tags: []string{"Webhooks"}}, h.listWebhookEvents)
	huma.Register(api, huma.Operation{
		OperationID: "createWebhook",
		Method:      "POST",
		Path:        "/api/webhooks",
		Summary:     "Create a webhook",
		Description: "Subscribes an HTTPS endpoint to workspace events (list them at `GET /api/webhooks/events`). " +
			"Deliveries are signed with the generated secret — verify that signature and reject anything that " +
			"fails it. Private and loopback targets are refused unless the operator has explicitly allowed them, " +
			"which is what stops a webhook being used to probe the instance's own network.",
		Tags:          []string{"Webhooks"},
		DefaultStatus: 201,
		Errors:        []int{400, 401, 403},
	}, h.createWebhook)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/webhooks/{id}", Summary: "Update Webhook", Tags: []string{"Webhooks"}}, h.updateWebhook)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/webhooks/{id}", Summary: "Delete Webhook", Tags: []string{"Webhooks"}}, h.deleteWebhook)

	// DNS providers, provider-accounts, domains, and DNS records are served by the
	// built-in dns Core plugin (plugins/dns), mounted by the app. See
	// website/src/content/docs/architecture/overview.md.

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/ai/assist/status", Summary: "Get AI Assist Status", Tags: []string{"AI"}}, h.aiStatus)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/ai/assist/suggest-slug", Summary: "Suggest Link Slug via AI", Tags: []string{"AI"}}, h.aiSuggestSlug)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/ai/assist/summarize-email/{id}", Summary: "Summarize Email via AI", Tags: []string{"AI"}}, h.aiSummarizeEmail)

	huma.Register(api, huma.Operation{
		OperationID: "listTokens",
		Method:      "GET",
		Path:        "/api/tokens",
		Summary:     "List API tokens",
		Description: "Lists the active workspace's API tokens. The secret is never returned here — it is shown " +
			"once, at creation. A token listed with no `lastUsedAt` has never authenticated a request and is " +
			"safe to delete.",
		Tags:   []string{"Tokens"},
		Errors: []int{401},
	}, h.listTokens)
	huma.Register(api, huma.Operation{
		OperationID: "createToken",
		Method:      "POST",
		Path:        "/api/tokens",
		Summary:     "Create an API token",
		Description: "Mints a bearer token for the active workspace and returns the secret **once** — it is stored " +
			"hashed and cannot be retrieved again. Send it as `Authorization: Bearer <token>`. A bearer-authenticated " +
			"request sends no cookies, so it is exempt from CSRF entirely.\n\n" +
			"The token inherits the creating user's role in the workspace; it does not outlive the workspace, and " +
			"deleting it takes effect on the next request.",
		Tags:          []string{"Tokens"},
		DefaultStatus: 201,
		Errors:        []int{401, 403},
	}, h.createToken)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/tokens/{id}", Summary: "Update API Token", Tags: []string{"Tokens"}}, h.updateToken)
	huma.Register(api, huma.Operation{
		OperationID: "deleteToken",
		Method:      "DELETE",
		Path:        "/api/tokens/{id}",
		Summary:     "Delete an API token",
		Description: "Revokes a token immediately: the next request presenting it answers 401. Deleting a token " +
			"that is already gone answers 404, so a retry after a lost response is distinguishable from a " +
			"double-revoke.",
		Tags:   []string{"Tokens"},
		Errors: []int{401, 404},
	}, h.deleteToken)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/notification-channel-types", Summary: "List Notification Channel Types", Tags: []string{"Notification Channels"}}, h.listNotificationChannelTypes)
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
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/org/slug", Summary: "Get Org Address", Tags: []string{"Org Management"}}, h.getOrgSlug)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/org/slug", Summary: "Change Org Address", Tags: []string{"Org Management"}}, h.updateOrgSlug)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/org/members", Summary: "List Org Members", Tags: []string{"Org Management"}}, h.listOrgMembers)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/org/members", Summary: "Add Org Member", Tags: []string{"Org Management"}}, h.addOrgMember)
	huma.Register(api, huma.Operation{Method: "PATCH", Path: "/api/org/members/{userId}", Summary: "Update Org Member Role", Tags: []string{"Org Management"}}, h.updateOrgMember)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/org/members/{userId}", Summary: "Remove Org Member", Tags: []string{"Org Management"}}, h.removeOrgMember)

	huma.Register(api, huma.Operation{
		OperationID: "exportAccount",
		Method:      "GET",
		Path:        "/api/account/export",
		Summary:     "Export workspace data",
		Description: "Streams everything the active workspace owns as a single JSON document — the data-portability " +
			"half of the GDPR pair whose other half is `DELETE /api/account/data`. Response size grows with the " +
			"workspace, so treat it as a download, not a synchronous API call.",
		Tags:   []string{"Account"},
		Errors: []int{401, 403},
	}, h.exportAccount)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/account/data", Summary: "Purge Org Data", Tags: []string{"Account"}}, h.purgeAccount)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/account/identities", Summary: "List Linked Identities", Tags: []string{"Account"}}, h.listIdentities)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/account/identities/{id}", Summary: "Unlink Identity", Tags: []string{"Account"}}, h.unlinkIdentity)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/account/user", Summary: "Delete My Account", Tags: []string{"Account"}}, h.deleteAccount)

	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/logout-all", Summary: "Logout All Sessions", Tags: []string{"Sessions"}}, h.logoutAll)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/sessions", Summary: "List Active Sessions", Tags: []string{"Sessions"}}, h.listSessions)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/auth/sessions/{id}", Summary: "Revoke Session", Tags: []string{"Sessions"}}, h.revokeSession)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/auth/2fa/status", Summary: "Get 2FA Status", Tags: []string{"2FA"}}, h.twoFAStatus)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/2fa/setup", Summary: "Setup 2FA", Tags: []string{"2FA"}}, h.setup2FA)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/2fa/enable", Summary: "Enable 2FA", Tags: []string{"2FA"}}, h.enable2FA)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/auth/2fa/disable", Summary: "Disable 2FA", Tags: []string{"2FA"}}, h.disable2FA)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/menus", Summary: "List Menu Toggles", Tags: []string{"UI Settings"}}, h.listMenus)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/actions", Summary: "List Plugin Actions", Tags: []string{"UI Settings"}}, h.listActions)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/plugins", Summary: "List Plugins", Tags: []string{"UI Settings"}}, h.listPlugins)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/plugins/{name}", Summary: "Toggle Plugin", Tags: []string{"UI Settings"}}, h.updatePlugin)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/user/settings", Summary: "Get User Settings", Tags: []string{"UI Settings"}}, h.getUserSettings)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/user/settings", Summary: "Update User Settings", Tags: []string{"UI Settings"}}, h.updateUserSettings)

	// /api/v1/x is a published alias for /api/x: the docs, the billing webhook
	// URLs and the inbound-mail webhook URLs all hand out the v1 form. Dispatch
	// happens through the same mux at request time, so routes plugins mount
	// after Routes() returns are reachable under the alias too.
	//
	// Anything classifying by path *before* the mux (the rate limiter's tierFor,
	// see internal/server/middleware.go) still sees the raw /api/v1/ path and
	// must normalize it itself — otherwise /api/v1/auth/login escapes the strict
	// auth tier.
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/" + strings.TrimPrefix(r.URL.Path, "/api/v1/")
		mux.ServeHTTP(w, r)
	})

	// The Error schema is registered by the first huma.Register above; fill in
	// the closed code registry now that it exists.
	documentErrorCodes(api.OpenAPI())

	return mux
}

// apiDescription is the spec's front matter. It is the first thing an
// integrator reads in the reference explorer, so it states the two things that
// decide whether their code survives our next release: what the base path is,
// and what an error looks like.
const apiDescription = `The Octarq REST API.

This document is generated from the Go handlers themselves — every path,
parameter and response below is registered code, not a hand-maintained
description of it. If it is not here, it is not served.

## Base path

All endpoints live under ` + "`/api/`" + `. ` + "`/api/v1/…`" + ` is accepted as an alias for
` + "`/api/…`" + `, but it is only a rewrite: it does not pin a frozen surface and
carries no compatibility guarantee the unversioned path does not. Prefer
` + "`/api/…`" + `.

## Authentication

Either the ` + "`octarq_session`" + ` cookie (dashboard, browser) or a bearer token
(` + "`Authorization: Bearer …`" + `, machine clients — see the Tokens endpoints).
Cookie-authenticated writes must also echo the CSRF token in ` + "`X-CSRF-Token`" + `.

## Errors

Every error, from every endpoint and every layer, is the same envelope:

    {"code": "validation_failed", "message": "…", "details": [...], "request_id": "…"}

Branch on ` + "`code`" + ` — it is a closed, documented set (see the Error schema).
Never match on ` + "`message`" + `: it is prose for humans and changes without notice.
Quote ` + "`request_id`" + ` to support and the request can be traced on our side.

## Deprecation

A deprecated operation is flagged ` + "`deprecated: true`" + ` here, carries an
` + "`x-sunset`" + ` date when one is announced, and emits ` + "`Deprecation`" + ` and
` + "`Sunset`" + ` response headers at runtime. Nothing is removed without that runway.`

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
	return h.origins.Absolute(0, r, origin.Secure(r, trustProxy))
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
