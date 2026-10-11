package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"gorm.io/gorm/clause"
)

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
	defAuth := defaultAuthRPM
	if v := os.Getenv("OCTARQ_RATELIMIT_AUTH_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			defAuth = n
		}
	}
	defAPI := defaultAPIRPM
	if v := os.Getenv("OCTARQ_RATELIMIT_API_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			defAPI = n
		}
	}
	defRedir := defaultRedirectRPM
	if v := os.Getenv("OCTARQ_RATELIMIT_REDIRECT_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			defRedir = n
		}
	}
	return h.settingInt(keyRatelimitAuthRPM, defAuth),
		h.settingInt(keyRatelimitAPIRPM, defAPI),
		h.settingInt(keyRatelimitRedirRPM, defRedir)
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
	if orgID == 0 {
		return ""
	}
	var s models.WorkspaceSetting
	if h.db.First(&s, "org_id = ? AND key = ?", orgID, key).Error == nil {
		return s.Value
	}
	return ""
}

func (h *Handler) SetWorkspaceSetting(orgID uint, key, value string) error {
	if orgID == 0 {
		return tenantsql.ErrZeroOrgID
	}
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

func (h *Handler) GetGlobalSetting(key string) string {
	return h.getSetting(key)
}

func (h *Handler) SetGlobalSetting(key, value string) error {
	return h.setSetting(key, value)
}
