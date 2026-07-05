package api

import (
	"net/http"
	"strconv"
	"strings"

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
)

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
func (h *Handler) isReservedMailbox(addr string) bool {
	local := strings.ToLower(addr)
	if at := strings.Index(local, "@"); at >= 0 {
		local = local[:at]
	}
	for _, r := range splitList(h.getSetting(keyReservedMailboxes)) {
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
		"reservedMailboxes":     h.getSetting(keyReservedMailboxes),
		"builtinReserved":       []string{"admin", "api", "assets", "portal"},
		"orgSlug":               org.Slug,
		"inboundToken":          org.InboundToken,
		"catchAll":              h.getSetting(keyCatchAll) == "true",
		"googleClientId":        h.getSetting(keyGoogleClientID),
		"googleClientSecretSet": h.getSetting(keyGoogleClientSecret) != "",
		"githubClientId":        h.getSetting(keyGitHubClientID),
		"githubClientSecretSet": h.getSetting(keyGitHubClientSecret) != "",
		"dataRetentionDays":     retDays,
		"autoWrapLinks":         h.getSetting(keyAutoWrapLinks) == "true",
		"allowRegistration":     h.registrationEnabled(),
	})
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	// These are instance-level secrets (OAuth client secrets, Cloudflare token,
	// catch-all, retention). Only org owners/admins may change them; a plain
	// member must not be able to rewrite the instance's auth or DNS config.
	if role := h.callerOrgRole(r); role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "owner or admin role required")
		return
	}
	var d struct {
		ReservedSlugs      *string `json:"reservedSlugs"`
		ReservedMailboxes  *string `json:"reservedMailboxes"`
		InboundToken       *string `json:"inboundToken"`
		CatchAll           *bool   `json:"catchAll"`
		GoogleClientID     *string `json:"googleClientId"`
		GoogleClientSecret *string `json:"googleClientSecret"` // "" clears, omitted keeps
		GitHubClientID     *string `json:"githubClientId"`
		GitHubClientSecret *string `json:"githubClientSecret"` // "" clears, omitted keeps
		DataRetentionDays  *int    `json:"dataRetentionDays"`  // 0 = disabled
		AutoWrapLinks      *bool   `json:"autoWrapLinks"`
		AllowRegistration  *bool   `json:"allowRegistration"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if d.ReservedSlugs != nil {
		h.setSetting(keyReservedSlugs, strings.Join(splitList(*d.ReservedSlugs), "\n"))
	}
	if d.ReservedMailboxes != nil {
		h.setSetting(keyReservedMailboxes, strings.Join(splitList(*d.ReservedMailboxes), "\n"))
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
		h.setSetting(keyCatchAll, val)
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
		h.setSetting(keyAutoWrapLinks, val)
	}
	if d.AllowRegistration != nil {
		val := "false"
		if *d.AllowRegistration {
			val = "true"
		}
		h.setSetting(keyAllowRegistration, val)
	}
	h.audit(r, "settings.update", "settings", 0, nil)
	h.getSettings(w, r)
}
