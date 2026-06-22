package api

import (
	"encoding/json"
	"errors"

	"github.com/jungley/led/internal/dnsprovider"
	"github.com/jungley/led/internal/models"
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

// providerFor decrypts a domain's stored credentials and builds its DNS provider.
func (h *Handler) providerFor(dom models.Domain) (dnsprovider.Provider, error) {
	if dom.Config == "" {
		return nil, errors.New("domain has no provider credentials configured")
	}
	creds, err := h.cipher.Decrypt(dom.Config)
	if err != nil {
		return nil, errors.New("decrypt provider credentials")
	}
	return dnsprovider.New(dom.Provider, creds)
}
