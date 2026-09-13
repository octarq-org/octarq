package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/authz"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/tenancy"
	"gorm.io/gorm/clause"
)

// currentOrg loads the caller's org, generating its inbound-webhook token on
// first read so the operator always has a token to copy into the worker URL.
func (h *Handler) currentOrg(r *http.Request) (models.Org, error) {
	if _, err := h.requireOrg(r); err != nil {
		return models.Org{}, err
	}
	var org models.Org
	h.db.First(&org, h.orgID(r))
	if org.ID != 0 && org.InboundToken == "" {
		org.InboundToken = uuid.NewString()
		h.db.Model(&org).Update("inbound_token", org.InboundToken)
	}
	return org, nil
}

// Setting keys.
const (
	keyReservedSlugs            = "reserved_slugs"
	keyReservedMailboxes        = "reserved_mailboxes"
	keyCatchAll                 = "catch_all"
	keyGoogleClientID           = "oauth.google.client_id"
	keyGoogleClientSecret       = "oauth.google.client_secret" // stored AES-GCM encrypted
	keyGitHubClientID           = "oauth.github.client_id"
	keyGitHubClientSecret       = "oauth.github.client_secret" // stored AES-GCM encrypted
	keyDataRetentionDays        = "data_retention_days"        // 0 = disabled
	keyAutoWrapLinks            = "auto_wrap_links"
	keyAllowRegistration        = "allow_registration" // "false" disables public sign-up; default on
	keyAppName                  = "app_name"           // UI product name; empty = config.DefaultAppName
	keyBrandLogo                = "brand_logo"         // white-label logo (URL or data URI); empty = gradient initial
	keyBrandColor               = "brand_color"        // white-label primary accent hex; empty = default indigo
	keyBrandColor2              = "brand_color_2"      // white-label secondary accent hex; empty = default violet
	keyMetricsToken             = "metrics_token"      // /metrics bearer; stored AES-GCM encrypted; empty = loopback-only
	keyRatelimitAuthRPM         = "ratelimit_auth_rpm"
	keyRatelimitAPIRPM          = "ratelimit_api_rpm"
	keyRatelimitRedirRPM        = "ratelimit_redirect_rpm"
	keyRequireEmailVerification = "require_email_verification" // "false" disables the email-verification requirement; default on
	keyBaseDomain               = models.BaseDomainSetting     // shared tenant-subdomain base; empty = feature off
	keySharedHosts              = models.SharedHostsSetting    // instance-wide shared hostnames; comma/newline-separated
	keyPublicCORSOrigins        = "public_cors_origins"        // comma/newline-separated exact origins allowed to read public GET endpoints
	keySystemSenderID           = "mail_system_sender_id"      // SMTPSender id used for instance-level system mail; empty = lowest-id sender
)

// Rate-limit defaults (requests per minute per IP) when the setting is unset.
const (
	defaultAuthRPM     = 60
	defaultAPIRPM      = 600
	defaultRedirectRPM = 6000
)

// Branding resolution
//
// Branding is per-workspace, with an instance-wide default underneath it, so one
// deployment can host many tenants that each look like their own product:
//
//	workspace setting (org) → instance setting → built-in default
//
// orgID 0 means "no workspace resolved" and skips straight to the instance
// layer. That is the honest answer on a shared host like app.octarq.org, where
// the login screen belongs to no tenant in particular.

// brandSetting resolves one branding key through the org → instance chain.
// A workspace that has never set the key inherits; a workspace that set it to
// empty also inherits (there is no "explicitly blank" branding).
func (h *Handler) brandSetting(orgID uint, key string) string {
	if orgID != 0 {
		if v := strings.TrimSpace(h.GetWorkspaceSetting(orgID, key)); v != "" {
			return v
		}
	}
	return strings.TrimSpace(h.getSetting(key))
}

// AppName returns the instance-default product name. Equivalent to
// AppNameFor(0); kept as the zero-argument form for callers with no workspace in
// hand.
func (h *Handler) AppName() string { return h.AppNameFor(0) }

// AppNameFor returns the product name shown to the given workspace, falling back
// to the instance name and then config.DefaultAppName.
func (h *Handler) AppNameFor(orgID uint) string {
	if v := h.brandSetting(orgID, keyAppName); v != "" {
		return v
	}
	return config.DefaultAppName
}

// Brand returns the instance-default white-label branding. Equivalent to
// BrandFor(0).
func (h *Handler) Brand() (logo, color, color2 string) { return h.BrandFor(0) }

// BrandFor returns the white-label branding (logo + accent colors) for the given
// workspace, falling back to the instance defaults. These keys have no core
// write path — they are set only by the Pro white-label plugin, so an OSS build
// always returns the zero values (default look). The values are surfaced
// publicly via GET /api/auth/config so the login screen and shell can theme
// before authentication.
func (h *Handler) BrandFor(orgID uint) (logo, color, color2 string) {
	return h.brandSetting(orgID, keyBrandLogo),
		h.brandSetting(orgID, keyBrandColor),
		h.brandSetting(orgID, keyBrandColor2)
}

// MetricsToken returns the decrypted /metrics bearer token; empty means the
// endpoint is loopback-only. Consumed by the edge middleware via a TTL cache.
func (h *Handler) MetricsToken() string {
	enc := h.getSetting(keyMetricsToken)
	if enc == "" {
		return ""
	}
	b, err := h.cipher.Decrypt(enc)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// RateLimits returns the per-IP requests-per-minute budgets (auth, api,
// redirect tiers) from settings, with defaults for unset/invalid values.
// A stored 0 or negative disables that tier's limiting.
func (h *Handler) RateLimits() (authRPM, apiRPM, redirectRPM int) {
	return h.settingInt(keyRatelimitAuthRPM, defaultAuthRPM),
		h.settingInt(keyRatelimitAPIRPM, defaultAPIRPM),
		h.settingInt(keyRatelimitRedirRPM, defaultRedirectRPM)
}

func (h *Handler) settingInt(key string, def int) int {
	v := strings.TrimSpace(h.getSetting(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// CORSOrigins returns the exact origins allowed to read public GET API
// endpoints cross-origin. The runtime settings table is the source of truth;
// the OCTARQ_CORS_ORIGINS env var is only a bootstrap fallback for a fresh
// instance that has not set the setting yet. An empty result means CORS is
// disabled entirely — no cross-origin reader is served CORS headers.
func (h *Handler) CORSOrigins() []string {
	if v := h.getSetting(keyPublicCORSOrigins); v != "" {
		return splitList(v)
	}
	return splitList(h.cfg.PublicCORSOrigins)
}

// registrationEnabled reports whether public email/password sign-up is allowed.
// Absent setting → enabled (default on); only an explicit "false" disables it.
func (h *Handler) registrationEnabled() bool {
	return h.getSetting(keyAllowRegistration) != "false"
}

// requireEmailVerification reports whether sign-up and login demand a verified
// email. Absent setting → required (default on); only an explicit "false"
// disables it. Multi-tenant instances default to requiring verification because
// unverified sign-ups are a mail-relay and abuse vector.
func (h *Handler) requireEmailVerification() bool {
	return h.getSetting(keyRequireEmailVerification) != "false"
}

// RequireEmailVerification is the public wrapper around requireEmailVerification,
// exposed to the app package so the startup readiness report reads the same
// setting the registration gate and the readiness API read.
func (h *Handler) RequireEmailVerification() bool {
	return h.requireEmailVerification()
}

// systemSenderID returns the instance setting naming which SMTPSender is the
// system sender; 0 means "unset — use the lowest-id sender".
func (h *Handler) systemSenderID() uint {
	v := strings.TrimSpace(h.getSetting(keySystemSenderID))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

// DefaultRetentionDays is used when no retention setting is configured.
const DefaultRetentionDays = 90

func (h *Handler) getSetting(key string) string {
	var s models.Setting
	if h.db.First(&s, "key = ?", key).Error == nil {
		return s.Value
	}
	return ""
}

func (h *Handler) setSetting(key, value string) error {
	return h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.Setting{Key: key, Value: value}).Error
}

func (h *Handler) GetWorkspaceSetting(orgID uint, key string) string {
	var s models.WorkspaceSetting
	if h.db.First(&s, "org_id = ? AND key = ?", orgID, key).Error == nil {
		return s.Value
	}
	return ""
}

func (h *Handler) SetWorkspaceSetting(orgID uint, key, value string) error {
	return h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.WorkspaceSetting{OrgID: orgID, Key: key, Value: value}).Error
}

// splitList parses a comma/newline/space-separated list into a normalized,
// lowercased, de-duplicated slice.
func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// --- handlers ---

type GetSettingsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetSettingsOutput struct {
	Body map[string]any
}

func (h *Handler) getSettings(ctx context.Context, input *GetSettingsInput) (*GetSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	org, err := h.currentOrg(r)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"reservedMailboxes": h.GetWorkspaceSetting(orgID, keyReservedMailboxes),
		"orgSlug":           org.Slug,
		// The org's automatic tenant subdomain, when a base domain is
		// configured; empty string = no base, so the org has only its slug.
		"tenantSubdomain": h.tenantSubdomain(org.Slug),
		"catchAll":        h.GetWorkspaceSetting(orgID, keyCatchAll) == "true",
		"autoWrapLinks":   h.GetWorkspaceSetting(orgID, keyAutoWrapLinks) == "true",
		"isInstanceAdmin": h.isInstanceAdmin(r),
	}
	// inboundTokenSet reports whether the org's inbound-email webhook secret is
	// set, without exposing it. Whoever holds the token can forge inbound mail
	// for the workspace, so the plaintext is never part of a settings dump — it
	// is fetched explicitly through the admin-only GET /api/settings/inbound-token
	// endpoint instead. Gate through callerHoldsRole rather than comparing
	// callerOrgRole directly — that is the one function that also understands API
	// tokens. Comparing the membership role alone hid the field from every
	// bearer-authenticated caller, since a token has no membership row.
	if h.callerHoldsRole(r, authz.RoleAdmin) {
		body["inboundTokenSet"] = org.InboundToken != ""
	}
	out := &GetSettingsOutput{
		Body: body,
	}
	return out, nil
}

type GetInboundTokenInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetInboundTokenInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetInboundTokenOutputBody struct {
	InboundToken string `json:"inboundToken"`
}

type GetInboundTokenOutput struct {
	Body GetInboundTokenOutputBody
}

// getInboundToken is the only endpoint that returns the org's inbound-email
// webhook secret in full. getSettings now answers with a boolean (inboundTokenSet),
// so the raw token never rides along in a settings dump; fetching it is an
// explicit, admin-or-owner-only action.
func (h *Handler) getInboundToken(ctx context.Context, input *GetInboundTokenInput) (*GetInboundTokenOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.callerHoldsRole(r, authz.RoleAdmin) {
		return nil, huma.Error403Forbidden("owner or admin role required")
	}
	if _, err := h.requireOrg(r); err != nil {
		return nil, err
	}
	org, err := h.currentOrg(r)
	if err != nil {
		return nil, err
	}
	out := &GetInboundTokenOutput{}
	out.Body.InboundToken = org.InboundToken
	return out, nil
}

// tenantSubdomain returns the org's automatic tenant address under the
// configured base domain, or "" when no base is configured. It is the read
// side of the provisioning on org creation, so the dashboard can show the
// address without importing the dns plugin.
func (h *Handler) tenantSubdomain(slug string) string {
	name, _ := tenancy.Subdomain(h.db, slug)
	return name
}

// isInstanceAdmin reports whether the current user is the bootstrap operator
// account (the user created from OCTARQ_ADMIN_*), and thus holds instance-level
// administrative privileges (/api/instance-settings).
//
// Privilege is bound to the stable User.IsInstanceAdmin flag — NOT to org_id
// ordering. The old "owner of org 1" check was a privilege-assignment vuln: on
// a fresh instance with registration/OAuth enabled, whoever registers first
// gets org 1 and would inherit instance admin. The flag is set deterministically
// for the configured admin account at first login (bootstrapUserID), so login
// order can never grant it to an attacker.
func (h *Handler) isInstanceAdmin(r *http.Request) bool {
	uid := h.auth.UserID(r)
	if uid == 0 {
		return false
	}
	var isAdmin bool
	if err := h.db.Model(&models.User{}).
		Where("id = ?", uid).
		Pluck("is_instance_admin", &isAdmin).Error; err != nil {
		return false
	}
	return isAdmin
}

// IsInstanceAdmin is the public wrapper around isInstanceAdmin, exposed to
// plugins via plugin.Context.IsInstanceAdmin. Instance-wide plugin config (SSO,
// the Pro license, instance branding defaults) must gate on this — on a
// multi-tenant host "is logged in" grants every tenant the operator's reach.
func (h *Handler) IsInstanceAdmin(r *http.Request) bool {
	return h.isInstanceAdmin(r)
}

type UpdateSettingsInputBody struct {
	ReservedMailboxes *string `json:"reservedMailboxes,omitempty"`
	InboundToken      *string `json:"inboundToken,omitempty"`
	CatchAll          *bool   `json:"catchAll,omitempty"`
	AutoWrapLinks     *bool   `json:"autoWrapLinks,omitempty"`
}

type UpdateSettingsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body UpdateSettingsInputBody
}

func (i *UpdateSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateSettingsOutput struct {
	Body map[string]any
}

func (h *Handler) updateSettings(ctx context.Context, input *UpdateSettingsInput) (*UpdateSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, huma.Error403Forbidden("owner or admin role required")
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}

	if input.Body.ReservedMailboxes != nil {
		h.SetWorkspaceSetting(orgID, keyReservedMailboxes, strings.Join(splitList(*input.Body.ReservedMailboxes), "\n"))
	}
	if input.Body.InboundToken != nil {
		// Per-org: empty string rotates to a fresh UUID; a value sets it explicitly.
		tok := strings.TrimSpace(*input.Body.InboundToken)
		if tok == "" {
			tok = uuid.NewString()
		}
		h.db.Model(&models.Org{}).Where("id = ?", orgID).Update("inbound_token", tok)
	}
	if input.Body.CatchAll != nil {
		val := "false"
		if *input.Body.CatchAll {
			val = "true"
		}
		h.SetWorkspaceSetting(orgID, keyCatchAll, val)
	}

	if input.Body.AutoWrapLinks != nil {
		val := "false"
		if *input.Body.AutoWrapLinks {
			val = "true"
		}
		h.SetWorkspaceSetting(orgID, keyAutoWrapLinks, val)
	}

	meta := make(map[string]any)
	if input.Body.ReservedMailboxes != nil {
		meta["reservedMailboxes"] = *input.Body.ReservedMailboxes
	}
	if input.Body.InboundToken != nil {
		meta["inboundToken"] = "[REDACTED]"
	}
	if input.Body.CatchAll != nil {
		meta["catchAll"] = *input.Body.CatchAll
	}
	if input.Body.AutoWrapLinks != nil {
		meta["autoWrapLinks"] = *input.Body.AutoWrapLinks
	}
	h.audit(r, "settings.update", "settings", 0, meta)

	org, err := h.currentOrg(r)
	if err != nil {
		return nil, err
	}
	out := &UpdateSettingsOutput{
		Body: map[string]any{
			"reservedMailboxes": h.GetWorkspaceSetting(org.ID, keyReservedMailboxes),
			"orgSlug":           org.Slug,
			"tenantSubdomain":   h.tenantSubdomain(org.Slug),
			"inboundTokenSet":   org.InboundToken != "",
			"catchAll":          h.GetWorkspaceSetting(org.ID, keyCatchAll) == "true",
			"autoWrapLinks":     h.GetWorkspaceSetting(org.ID, keyAutoWrapLinks) == "true",
			"isInstanceAdmin":   h.isInstanceAdmin(r),
		},
	}
	return out, nil
}

func (h *Handler) GetGlobalSetting(key string) string {
	return h.getSetting(key)
}

func (h *Handler) SetGlobalSetting(key, value string) error {
	return h.setSetting(key, value)
}
