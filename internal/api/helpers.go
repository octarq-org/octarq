package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Jungley8/led/internal/dnsprovider"
	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
)

var errNotFound = errors.New("not found")

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
// provider. Cloudflare domains without their own credentials fall back to the
// global Cloudflare token configured in Settings.
func (h *Handler) providerFor(dom models.Domain) (dnsprovider.Provider, error) {
	if dom.ProviderAccountID == 0 {
		return nil, errors.New("domain has no provider account configured")
	}
	var acc models.ProviderAccount
	if err := h.db.First(&acc, dom.ProviderAccountID).Error; err != nil {
		return nil, errors.New("provider account not found")
	}
	if acc.Config == "" {
		if acc.Type == "cloudflare" {
			if tok := h.cloudflareToken(); tok != "" {
				creds, _ := json.Marshal(map[string]string{"apiToken": tok})
				return dnsprovider.New("cloudflare", creds)
			}
		}
		return nil, errors.New("provider account has no credentials configured")
	}
	creds, err := h.cipher.Decrypt(acc.Config)
	if err != nil {
		return nil, errors.New("decrypt provider credentials")
	}
	return dnsprovider.New(acc.Type, creds)
}
