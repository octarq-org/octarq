package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

var errNotFound = errors.New("not found")

// secureEqual reports whether a and b are equal using a constant-time
// comparison, avoiding timing side channels when checking secret tokens.
func secureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// trustProxy gates whether proxy-supplied client-IP headers are honoured. Set
// once from config in New; when false, a client cannot spoof X-Forwarded-For
// to get a fresh abuse-report rate-limit bucket.
var trustProxy bool

// reporterIP returns the best-guess client IP for abuse reports.
// We keep the full IP here (unlike analytics) so admins can block repeat abusers.
func reporterIP(r *http.Request) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
		if rip := r.Header.Get("X-Real-IP"); rip != "" {
			return rip
		}
	}
	host := r.RemoteAddr
	if h, _, err := splitHostPort(host); err == nil {
		return h
	}
	return host
}

func splitHostPort(addr string) (host, port string, err error) {
	// Thin wrapper so abuse.go doesn't import "net" directly.
	import_net_SplitHostPort := func(hostport string) (string, string, error) {
		// inline net.SplitHostPort to keep imports clean
		for i := len(hostport) - 1; i >= 0; i-- {
			if hostport[i] == ':' {
				return hostport[:i], hostport[i+1:], nil
			}
		}
		return "", "", errors.New("no port")
	}
	return import_net_SplitHostPort(addr)
}

// emailBelongsToOrg verifies an email's mailbox is owned by the given org.
func (h *Handler) emailBelongsToOrg(emailID, orgID uint) bool {
	var count int64
	h.db.Model(&models.Email{}).
		Joins("JOIN mailboxes ON mailboxes.id = emails.mailbox_id AND mailboxes.owner_id = ?", orgID).
		Where("emails.id = ?", emailID).Count(&count)
	return count > 0
}

// orgID extracts the authenticated org ID from the request session.
// Falls back to 1 (the bootstrap org) if the session predates multi-tenant.
func (h *Handler) orgID(r *http.Request) uint {
	if id := h.auth.OrgID(r); id != 0 {
		return id
	}
	return 1
}

// orgDB returns a *gorm.DB pre-scoped to the authenticated org.
// Use instead of h.db for any query that should be tenant-isolated.
func (h *Handler) orgDB(r *http.Request) *gorm.DB {
	return h.db.Where("owner_id = ?", h.orgID(r))
}

// audit writes an AuditLog entry asynchronously; never blocks a request.
// meta is an optional map that is JSON-encoded (pass nil to omit).
func (h *Handler) audit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	orgID := h.orgID(r)
	actorID := h.auth.UserID(r)
	ip := reporterIP(r)
	var metaJSON string
	if meta != nil {
		if b, err := json.Marshal(meta); err == nil {
			metaJSON = string(b)
		}
	}
	go func() {
		h.db.Create(&models.AuditLog{
			OrgID:      orgID,
			ActorID:    actorID,
			Action:     action,
			TargetType: targetType,
			TargetID:   targetID,
			Meta:       metaJSON,
			IP:         ip,
		})
	}()
}

// Audit is a public wrapper around audit, exposed for plugins via plugin.Context.
func (h *Handler) Audit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	h.audit(r, action, targetType, targetID, meta)
}

// encryptConfig serializes and AES-GCM-encrypts a provider credentials map.
func (h *Handler) encryptConfig(cfg map[string]any) (string, error) {
	if len(cfg) == 0 {
		return "", nil
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return h.cipher.Encrypt(b)
}

// providerFor decrypts a domain's stored credentials and builds its DNS
// provider.
func (h *Handler) providerFor(dom models.Domain) (dnsprovider.Provider, error) {
	if dom.ProviderAccountID == 0 {
		return nil, errors.New("domain has no provider account configured")
	}
	// Scope the provider-account lookup to the domain's owning org. Callers
	// already pre-scope the Domain, so this is defense-in-depth: it prevents a
	// domain from ever resolving credentials belonging to another tenant even
	// if a future caller forgets to scope the domain first.
	var acc models.ProviderAccount
	if err := h.db.Where("id = ? AND owner_id = ?", dom.ProviderAccountID, dom.OrgID).
		First(&acc).Error; err != nil {
		return nil, errors.New("provider account not found")
	}
	if acc.Config == "" {
		return nil, errors.New("provider account has no credentials configured")
	}
	creds, err := h.cipher.Decrypt(acc.Config)
	if err != nil {
		return nil, errors.New("stored API token could not be decrypted — re-save this provider's API token under Settings → DNS Providers (the encryption key or database changed since it was saved)")
	}
	return dnsprovider.New(acc.Type, creds)
}
