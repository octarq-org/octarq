package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/models"
	"github.com/google/uuid"
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

func (h *Handler) getWorkspaceSetting(orgID uint, key string) string {
	var s models.WorkspaceSetting
	if h.db.First(&s, "org_id = ? AND key = ?", orgID, key).Error == nil {
		return s.Value
	}
	return ""
}

func (h *Handler) setWorkspaceSetting(orgID uint, key, value string) error {
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
	for _, r := range splitList(h.getWorkspaceSetting(orgID, keyReservedMailboxes)) {
		if r == local {
			return true
		}
	}
	return false
}

// --- handlers ---

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	retDays := DefaultRetentionDays
	if v := h.getSetting(keyDataRetentionDays); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			retDays = n
		}
	}
	org := h.currentOrg(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"reservedSlugs":         h.getSetting(keyReservedSlugs),
		"reservedMailboxes":     h.getWorkspaceSetting(org.ID, keyReservedMailboxes),
		"builtinReserved":       []string{"admin", "api", "assets", "portal"},
		"orgSlug":               org.Slug,
		"inboundToken":          org.InboundToken,
		"catchAll":              h.getWorkspaceSetting(org.ID, keyCatchAll) == "true",
		"googleClientId":        h.getSetting(keyGoogleClientID),
		"googleClientSecretSet": h.getSetting(keyGoogleClientSecret) != "",
		"githubClientId":        h.getSetting(keyGitHubClientID),
		"githubClientSecretSet": h.getSetting(keyGitHubClientSecret) != "",
		"dataRetentionDays":     retDays,
		"autoWrapLinks":         h.getWorkspaceSetting(org.ID, keyAutoWrapLinks) == "true",
		"allowRegistration":     h.registrationEnabled(),
		"appName":               h.getSetting(keyAppName), // raw value; empty = default
		"metricsTokenSet":       h.getSetting(keyMetricsToken) != "",
		"ratelimitAuthRpm":      h.settingInt(keyRatelimitAuthRPM, defaultAuthRPM),
		"ratelimitApiRpm":       h.settingInt(keyRatelimitAPIRPM, defaultAPIRPM),
		"ratelimitRedirectRpm":  h.settingInt(keyRatelimitRedirRPM, defaultRedirectRPM),
	})
}

// isInstanceAdmin reports whether the current user is an owner of the bootstrap org (org 1),
// and thus holds instance-level administrative privileges.
func (h *Handler) isInstanceAdmin(r *http.Request) bool {
	uid := h.auth.UserID(r)
	if uid == 0 {
		return false
	}
	var role string
	if err := h.db.Model(&models.OrgMember{}).
		Where("org_id = ? AND user_id = ?", 1, uid).
		Pluck("role", &role).Error; err != nil {
		return false
	}
	return role == "owner"
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	// Base permission: Only org owners/admins may change settings for their org.
	// Instance-level settings have an additional check below requiring instance admin.
	if role := h.callerOrgRole(r); role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "owner or admin role required")
		return
	}
	var d struct {
		ReservedSlugs        *string `json:"reservedSlugs"`
		ReservedMailboxes    *string `json:"reservedMailboxes"`
		InboundToken         *string `json:"inboundToken"`
		CatchAll             *bool   `json:"catchAll"`
		GoogleClientID       *string `json:"googleClientId"`
		GoogleClientSecret   *string `json:"googleClientSecret"` // "" clears, omitted keeps
		GitHubClientID       *string `json:"githubClientId"`
		GitHubClientSecret   *string `json:"githubClientSecret"` // "" clears, omitted keeps
		DataRetentionDays    *int    `json:"dataRetentionDays"`  // 0 = disabled
		AutoWrapLinks        *bool   `json:"autoWrapLinks"`
		AllowRegistration    *bool   `json:"allowRegistration"`
		AppName              *string `json:"appName"`      // "" resets to the built-in default
		MetricsToken         *string `json:"metricsToken"` // "" clears (loopback-only), omitted keeps
		RatelimitAuthRpm     *int    `json:"ratelimitAuthRpm"`
		RatelimitApiRpm      *int    `json:"ratelimitApiRpm"`
		RatelimitRedirectRpm *int    `json:"ratelimitRedirectRpm"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	hasInstanceUpdates := d.ReservedSlugs != nil || d.GoogleClientID != nil ||
		d.GoogleClientSecret != nil || d.GitHubClientID != nil ||
		d.GitHubClientSecret != nil || d.DataRetentionDays != nil ||
		d.AllowRegistration != nil || d.AppName != nil ||
		d.MetricsToken != nil || d.RatelimitAuthRpm != nil ||
		d.RatelimitApiRpm != nil || d.RatelimitRedirectRpm != nil

	if hasInstanceUpdates && !h.isInstanceAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin role required to update instance settings")
		return
	}
	if d.ReservedSlugs != nil {
		h.setSetting(keyReservedSlugs, strings.Join(splitList(*d.ReservedSlugs), "\n"))
	}
	if d.ReservedMailboxes != nil {
		h.setWorkspaceSetting(h.orgID(r), keyReservedMailboxes, strings.Join(splitList(*d.ReservedMailboxes), "\n"))
	}
	if d.InboundToken != nil {
		// Per-org: empty string rotates to a fresh UUID; a value sets it explicitly.
		tok := strings.TrimSpace(*d.InboundToken)
		if tok == "" {
			tok = uuid.NewString()
		}
		h.db.Model(&models.Org{}).Where("id = ?", h.orgID(r)).Update("inbound_token", tok)
	}
	if d.CatchAll != nil {
		val := "false"
		if *d.CatchAll {
			val = "true"
		}
		h.setWorkspaceSetting(h.orgID(r), keyCatchAll, val)
	}
	if d.GoogleClientID != nil {
		h.setSetting(keyGoogleClientID, strings.TrimSpace(*d.GoogleClientID))
	}
	if d.GoogleClientSecret != nil {
		if *d.GoogleClientSecret == "" {
			h.setSetting(keyGoogleClientSecret, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*d.GoogleClientSecret)))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "encrypt token")
				return
			}
			h.setSetting(keyGoogleClientSecret, enc)
		}
	}
	if d.GitHubClientID != nil {
		h.setSetting(keyGitHubClientID, strings.TrimSpace(*d.GitHubClientID))
	}
	if d.GitHubClientSecret != nil {
		if *d.GitHubClientSecret == "" {
			h.setSetting(keyGitHubClientSecret, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*d.GitHubClientSecret)))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "encrypt token")
				return
			}
			h.setSetting(keyGitHubClientSecret, enc)
		}
	}
	if d.DataRetentionDays != nil {
		h.setSetting(keyDataRetentionDays, strconv.Itoa(*d.DataRetentionDays))
	}
	if d.AutoWrapLinks != nil {
		val := "false"
		if *d.AutoWrapLinks {
			val = "true"
		}
		h.setWorkspaceSetting(h.orgID(r), keyAutoWrapLinks, val)
	}
	if d.AllowRegistration != nil {
		val := "false"
		if *d.AllowRegistration {
			val = "true"
		}
		h.setSetting(keyAllowRegistration, val)
	}
	if d.AppName != nil {
		h.setSetting(keyAppName, strings.TrimSpace(*d.AppName))
	}
	if d.MetricsToken != nil {
		if *d.MetricsToken == "" {
			h.setSetting(keyMetricsToken, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*d.MetricsToken)))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "encrypt token")
				return
			}
			h.setSetting(keyMetricsToken, enc)
		}
	}
	if d.RatelimitAuthRpm != nil {
		h.setSetting(keyRatelimitAuthRPM, strconv.Itoa(*d.RatelimitAuthRpm))
	}
	if d.RatelimitApiRpm != nil {
		h.setSetting(keyRatelimitAPIRPM, strconv.Itoa(*d.RatelimitApiRpm))
	}
	if d.RatelimitRedirectRpm != nil {
		h.setSetting(keyRatelimitRedirRPM, strconv.Itoa(*d.RatelimitRedirectRpm))
	}
	h.audit(r, "settings.update", "settings", 0, nil)
	h.getSettings(w, r)
}
