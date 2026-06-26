package api

import (
	"encoding/json"
	"errors"

	"github.com/Jungley8/led/internal/dnsprovider"
	"github.com/Jungley8/led/internal/models"
)

var errNotFound = errors.New("not found")

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
