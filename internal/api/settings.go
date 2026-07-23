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
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm/clause"
)

// currentOrg loads the caller's org, generating its inbound-webhook token on
// first read so the operator always has a token to copy into the worker URL.
func (h *Handler) currentOrg(r *http.Request) models.Org {
	var org models.Org
	h.db.First(&org, h.orgID(r))
	if org.ID != 0 && org.InboundToken == "" {
		org.InboundToken = uuid.NewString()
		h.db.Model(&org).Update("inbound_token", org.InboundToken)
	}
	return org
}

// Setting keys.
const (
	keyReservedSlugs      = "reserved_slugs"
	keyReservedMailboxes  = "reserved_mailboxes"
	keyCatchAll           = "catch_all"
	keyGoogleClientID     = "oauth.google.client_id"
	keyGoogleClientSecret = "oauth.google.client_secret" // stored AES-GCM encrypted
	keyGitHubClientID     = "oauth.github.client_id"
	keyGitHubClientSecret = "oauth.github.client_secret" // stored AES-GCM encrypted
	keyDataRetentionDays  = "data_retention_days"        // 0 = disabled
	keyAutoWrapLinks      = "auto_wrap_links"
	keyAllowRegistration  = "allow_registration" // "false" disables public sign-up; default on
	keyAppName            = "app_name"           // UI product name; empty = config.DefaultAppName
	keyBrandLogo          = "brand_logo"         // white-label logo (URL or data URI); empty = gradient initial
	keyBrandColor         = "brand_color"        // white-label primary accent hex; empty = default indigo
	keyBrandColor2        = "brand_color_2"      // white-label secondary accent hex; empty = default violet
	keyMetricsToken       = "metrics_token"      // /metrics bearer; stored AES-GCM encrypted; empty = loopback-only
	keyRatelimitAuthRPM   = "ratelimit_auth_rpm"
	keyRatelimitAPIRPM    = "ratelimit_api_rpm"
	keyRatelimitRedirRPM  = "ratelimit_redirect_rpm"
)

// Rate-limit defaults (requests per minute per IP) when the setting is unset.
const (
	defaultAuthRPM     = 60
	defaultAPIRPM      = 600
	defaultRedirectRPM = 6000
)

// AppName returns the runtime product name (Settings → General), falling back
// to config.DefaultAppName.
func (h *Handler) AppName() string {
	if v := strings.TrimSpace(h.getSetting(keyAppName)); v != "" {
		return v
	}
	return config.DefaultAppName
}

// Brand returns the runtime white-label branding (logo + accent colors) from
// Settings. These keys have no core write path — they are set only by the Pro
// white-label plugin, so an OSS build always returns the zero values (default
// look). The values are surfaced publicly via GET /api/auth/config so the login
// screen and shell can theme before authentication.
func (h *Handler) Brand() (logo, color, color2 string) {
	return strings.TrimSpace(h.getSetting(keyBrandLogo)),
		strings.TrimSpace(h.getSetting(keyBrandColor)),
		strings.TrimSpace(h.getSetting(keyBrandColor2))
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

// registrationEnabled reports whether public email/password sign-up is allowed.
// Absent setting → enabled (default on); only an explicit "false" disables it.
func (h *Handler) registrationEnabled() bool {
	return h.getSetting(keyAllowRegistration) != "false"
}

// DefaultRetentionDays is used when no retention setting is configured.
const DefaultRetentionDays = 90

// Slugs that can never be short links because they collide with reserved
// top-level routes.
var builtinReservedSlugs = map[string]bool{"admin": true, "api": true, "assets": true, "portal": true}

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

// isReservedSlug reports whether a slug may not be used for a short link.
func (h *Handler) isReservedSlug(slug string) bool {
	slug = strings.ToLower(slug)
	if builtinReservedSlugs[slug] {
		return true
	}
	for _, r := range splitList(h.getSetting(keyReservedSlugs)) {
		if r == slug {
			return true
		}
	}
	return false
}

// isReservedMailbox reports whether a mailbox local-part is reserved (excluded
// from catch-all auto-creation).
func (h *Handler) isReservedMailbox(orgID uint, addr string) bool {
	local := strings.ToLower(addr)
	if at := strings.Index(local, "@"); at >= 0 {
		local = local[:at]
	}
	for _, r := range splitList(h.GetWorkspaceSetting(orgID, keyReservedMailboxes)) {
		if r == local {
			return true
		}
	}
	return false
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
	org := h.currentOrg(r)
	out := &GetSettingsOutput{
		Body: map[string]any{
			"reservedMailboxes": h.GetWorkspaceSetting(org.ID, keyReservedMailboxes),
			"orgSlug":           org.Slug,
			"inboundToken":      org.InboundToken,
			"catchAll":          h.GetWorkspaceSetting(org.ID, keyCatchAll) == "true",
			"autoWrapLinks":     h.GetWorkspaceSetting(org.ID, keyAutoWrapLinks) == "true",
			"isInstanceAdmin":   h.isInstanceAdmin(r),
		},
	}
	return out, nil
}

type GetInstanceSettingsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetInstanceSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetInstanceSettingsOutput struct {
	Body map[string]any
}

func (h *Handler) getInstanceSettings(ctx context.Context, input *GetInstanceSettingsInput) (*GetInstanceSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("instance admin role required")
	}
	retDays := DefaultRetentionDays
	if v := h.getSetting(keyDataRetentionDays); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retDays = n
		}
	}
	out := &GetInstanceSettingsOutput{
		Body: map[string]any{
			"reservedSlugs":         h.getSetting(keyReservedSlugs),
			"builtinReserved":       []string{"admin", "api", "assets", "portal"},
			"googleClientId":        h.getSetting(keyGoogleClientID),
			"googleClientSecretSet": h.getSetting(keyGoogleClientSecret) != "",
			"githubClientId":        h.getSetting(keyGitHubClientID),
			"githubClientSecretSet": h.getSetting(keyGitHubClientSecret) != "",
			"dataRetentionDays":     retDays,
			"allowRegistration":     h.registrationEnabled(),
			"appName":               h.getSetting(keyAppName), // raw value; empty = default
			"metricsTokenSet":       h.getSetting(keyMetricsToken) != "",
			"ratelimitAuthRpm":      h.settingInt(keyRatelimitAuthRPM, defaultAuthRPM),
			"ratelimitApiRpm":       h.settingInt(keyRatelimitAPIRPM, defaultAPIRPM),
			"ratelimitRedirectRpm":  h.settingInt(keyRatelimitRedirRPM, defaultRedirectRPM),
		},
	}
	return out, nil
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

type UpdateSettingsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		ReservedMailboxes *string `json:"reservedMailboxes,omitempty"`
		InboundToken      *string `json:"inboundToken,omitempty"`
		CatchAll          *bool   `json:"catchAll,omitempty"`
		AutoWrapLinks     *bool   `json:"autoWrapLinks,omitempty"`
	}
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
	if role := h.callerOrgRole(r); role != "owner" && role != "admin" {
		return nil, huma.Error403Forbidden("owner or admin role required")
	}

	if input.Body.ReservedMailboxes != nil {
		h.SetWorkspaceSetting(h.orgID(r), keyReservedMailboxes, strings.Join(splitList(*input.Body.ReservedMailboxes), "\n"))
	}
	if input.Body.InboundToken != nil {
		// Per-org: empty string rotates to a fresh UUID; a value sets it explicitly.
		tok := strings.TrimSpace(*input.Body.InboundToken)
		if tok == "" {
			tok = uuid.NewString()
		}
		h.db.Model(&models.Org{}).Where("id = ?", h.orgID(r)).Update("inbound_token", tok)
	}
	if input.Body.CatchAll != nil {
		val := "false"
		if *input.Body.CatchAll {
			val = "true"
		}
		h.SetWorkspaceSetting(h.orgID(r), keyCatchAll, val)
	}

	if input.Body.AutoWrapLinks != nil {
		val := "false"
		if *input.Body.AutoWrapLinks {
			val = "true"
		}
		h.SetWorkspaceSetting(h.orgID(r), keyAutoWrapLinks, val)
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

	org := h.currentOrg(r)
	out := &UpdateSettingsOutput{
		Body: map[string]any{
			"reservedMailboxes": h.GetWorkspaceSetting(org.ID, keyReservedMailboxes),
			"orgSlug":           org.Slug,
			"inboundToken":      org.InboundToken,
			"catchAll":          h.GetWorkspaceSetting(org.ID, keyCatchAll) == "true",
			"autoWrapLinks":     h.GetWorkspaceSetting(org.ID, keyAutoWrapLinks) == "true",
			"isInstanceAdmin":   h.isInstanceAdmin(r),
		},
	}
	return out, nil
}

type UpdateInstanceSettingsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		ReservedSlugs        *string `json:"reservedSlugs,omitempty"`
		GoogleClientID       *string `json:"googleClientId,omitempty"`
		GoogleClientSecret   *string `json:"googleClientSecret,omitempty"`
		GitHubClientID       *string `json:"githubClientId,omitempty"`
		GitHubClientSecret   *string `json:"githubClientSecret,omitempty"`
		DataRetentionDays    *int    `json:"dataRetentionDays,omitempty"`
		AllowRegistration    *bool   `json:"allowRegistration,omitempty"`
		AppName              *string `json:"appName,omitempty"`
		MetricsToken         *string `json:"metricsToken,omitempty"`
		RatelimitAuthRpm     *int    `json:"ratelimitAuthRpm,omitempty"`
		RatelimitApiRpm      *int    `json:"ratelimitApiRpm,omitempty"`
		RatelimitRedirectRpm *int    `json:"ratelimitRedirectRpm,omitempty"`
	}
}

func (i *UpdateInstanceSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateInstanceSettingsOutput struct {
	Body map[string]any
}

func (h *Handler) updateInstanceSettings(ctx context.Context, input *UpdateInstanceSettingsInput) (*UpdateInstanceSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("instance admin role required")
	}

	if input.Body.ReservedSlugs != nil {
		h.setSetting(keyReservedSlugs, strings.Join(splitList(*input.Body.ReservedSlugs), "\n"))
	}
	if input.Body.GoogleClientID != nil {
		h.setSetting(keyGoogleClientID, strings.TrimSpace(*input.Body.GoogleClientID))
	}
	if input.Body.GoogleClientSecret != nil {
		if *input.Body.GoogleClientSecret == "" {
			h.setSetting(keyGoogleClientSecret, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*input.Body.GoogleClientSecret)))
			if err != nil {
				return nil, huma.Error500InternalServerError("encrypt token")
			}
			h.setSetting(keyGoogleClientSecret, enc)
		}
	}
	if input.Body.GitHubClientID != nil {
		h.setSetting(keyGitHubClientID, strings.TrimSpace(*input.Body.GitHubClientID))
	}
	if input.Body.GitHubClientSecret != nil {
		if *input.Body.GitHubClientSecret == "" {
			h.setSetting(keyGitHubClientSecret, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*input.Body.GitHubClientSecret)))
			if err != nil {
				return nil, huma.Error500InternalServerError("encrypt token")
			}
			h.setSetting(keyGitHubClientSecret, enc)
		}
	}
	if input.Body.DataRetentionDays != nil {
		h.setSetting(keyDataRetentionDays, strconv.Itoa(*input.Body.DataRetentionDays))
	}
	if input.Body.AllowRegistration != nil {
		val := "false"
		if *input.Body.AllowRegistration {
			val = "true"
		}
		h.setSetting(keyAllowRegistration, val)
	}
	if input.Body.AppName != nil {
		h.setSetting(keyAppName, strings.TrimSpace(*input.Body.AppName))
	}
	if input.Body.MetricsToken != nil {
		if *input.Body.MetricsToken == "" {
			h.setSetting(keyMetricsToken, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*input.Body.MetricsToken)))
			if err != nil {
				return nil, huma.Error500InternalServerError("encrypt token")
			}
			h.setSetting(keyMetricsToken, enc)
		}
	}
	if input.Body.RatelimitAuthRpm != nil {
		h.setSetting(keyRatelimitAuthRPM, strconv.Itoa(*input.Body.RatelimitAuthRpm))
	}
	if input.Body.RatelimitApiRpm != nil {
		h.setSetting(keyRatelimitAPIRPM, strconv.Itoa(*input.Body.RatelimitApiRpm))
	}
	meta := make(map[string]any)
	if input.Body.ReservedSlugs != nil {
		meta["reservedSlugs"] = *input.Body.ReservedSlugs
	}
	if input.Body.GoogleClientID != nil {
		meta["googleClientId"] = *input.Body.GoogleClientID
	}
	if input.Body.GoogleClientSecret != nil {
		meta["googleClientSecret"] = "[REDACTED]"
	}
	if input.Body.GitHubClientID != nil {
		meta["githubClientId"] = *input.Body.GitHubClientID
	}
	if input.Body.GitHubClientSecret != nil {
		meta["githubClientSecret"] = "[REDACTED]"
	}
	if input.Body.DataRetentionDays != nil {
		meta["dataRetentionDays"] = *input.Body.DataRetentionDays
	}
	if input.Body.AllowRegistration != nil {
		meta["allowRegistration"] = *input.Body.AllowRegistration
	}
	if input.Body.AppName != nil {
		meta["appName"] = *input.Body.AppName
	}
	if input.Body.MetricsToken != nil {
		meta["metricsToken"] = "[REDACTED]"
	}
	if input.Body.RatelimitAuthRpm != nil {
		meta["ratelimitAuthRpm"] = *input.Body.RatelimitAuthRpm
	}
	if input.Body.RatelimitApiRpm != nil {
		meta["ratelimitApiRpm"] = *input.Body.RatelimitApiRpm
	}
	if input.Body.RatelimitRedirectRpm != nil {
		meta["ratelimitRedirectRpm"] = *input.Body.RatelimitRedirectRpm
	}
	h.audit(r, "instance_settings.update", "settings", 0, meta)

	retDays := DefaultRetentionDays
	if v := h.getSetting(keyDataRetentionDays); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retDays = n
		}
	}
	out := &UpdateInstanceSettingsOutput{
		Body: map[string]any{
			"reservedSlugs":         h.getSetting(keyReservedSlugs),
			"builtinReserved":       []string{"admin", "api", "assets", "portal"},
			"googleClientId":        h.getSetting(keyGoogleClientID),
			"googleClientSecretSet": h.getSetting(keyGoogleClientSecret) != "",
			"githubClientId":        h.getSetting(keyGitHubClientID),
			"githubClientSecretSet": h.getSetting(keyGitHubClientSecret) != "",
			"dataRetentionDays":     retDays,
			"allowRegistration":     h.registrationEnabled(),
			"appName":               h.getSetting(keyAppName),
			"metricsTokenSet":       h.getSetting(keyMetricsToken) != "",
			"ratelimitAuthRpm":      h.settingInt(keyRatelimitAuthRPM, defaultAuthRPM),
			"ratelimitApiRpm":       h.settingInt(keyRatelimitAPIRPM, defaultAPIRPM),
			"ratelimitRedirectRpm":  h.settingInt(keyRatelimitRedirRPM, defaultRedirectRPM),
		},
	}
	return out, nil
}

func (h *Handler) GetGlobalSetting(key string) string {
	return h.getSetting(key)
}
