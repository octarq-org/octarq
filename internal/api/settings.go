package api

import (
	"net/http"
	"strings"

	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm/clause"
)

// Setting keys.
const (
	keyReservedSlugs     = "reserved_slugs"
	keyReservedMailboxes = "reserved_mailboxes"
	keyCloudflareToken   = "cloudflare_token" // stored AES-GCM encrypted
	keyInboundToken      = "inbound_token"
	keyCatchAll          = "catch_all"
)

// Slugs that can never be short links because they collide with reserved
// top-level routes.
var builtinReservedSlugs = map[string]bool{"admin": true, "api": true, "assets": true}

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

// cloudflareToken returns the decrypted global Cloudflare token, if configured.
func (h *Handler) cloudflareToken() string {
	enc := h.getSetting(keyCloudflareToken)
	if enc == "" {
		return ""
	}
	b, err := h.cipher.Decrypt(enc)
	if err != nil {
		return ""
	}
	return string(b)
}

// --- handlers ---

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"reservedSlugs":      h.getSetting(keyReservedSlugs),
		"reservedMailboxes":  h.getSetting(keyReservedMailboxes),
		"builtinReserved":    []string{"admin", "api", "assets"},
		"cloudflareTokenSet": h.getSetting(keyCloudflareToken) != "",
		"inboundToken":       h.getSetting(keyInboundToken),
		"catchAll":           h.getSetting(keyCatchAll) == "true",
	})
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var d struct {
		ReservedSlugs     *string `json:"reservedSlugs"`
		ReservedMailboxes *string `json:"reservedMailboxes"`
		CloudflareToken   *string `json:"cloudflareToken"` // "" clears, omitted keeps
		InboundToken      *string `json:"inboundToken"`
		CatchAll          *bool   `json:"catchAll"`
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
	if d.CloudflareToken != nil {
		if *d.CloudflareToken == "" {
			h.setSetting(keyCloudflareToken, "")
		} else {
			enc, err := h.cipher.Encrypt([]byte(strings.TrimSpace(*d.CloudflareToken)))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "encrypt token")
				return
			}
			h.setSetting(keyCloudflareToken, enc)
		}
	}
	if d.InboundToken != nil {
		h.setSetting(keyInboundToken, strings.TrimSpace(*d.InboundToken))
	}
	if d.CatchAll != nil {
		val := "false"
		if *d.CatchAll {
			val = "true"
		}
		h.setSetting(keyCatchAll, val)
	}
	h.getSettings(w, r)
}
