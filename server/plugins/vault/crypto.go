package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/octarq-org/octarq/server/plugin"
)

func itemSubject(secretID string) string {
	return "vault.item." + secretID
}

func itemAAD(orgID uint, secretID string, version int) []byte {
	return []byte(fmt.Sprintf("vault:v1:%d:%s:%d", orgID, secretID, version))
}

func (p *Plugin) requireCrypto() (plugin.CryptoVault, error) {
	if p.host == nil || p.host.Crypto() == nil {
		return nil, errors.New("vault: cryptographic subsystem unavailable")
	}
	return p.host.Crypto(), nil
}

func (p *Plugin) sealPayload(orgID uint, secretID string, version int, payload map[string]any) (string, error) {
	c, err := p.requireCrypto()
	if err != nil {
		return "", err
	}
	sv, err := c.SubjectVault(orgID, itemSubject(secretID))
	if err != nil {
		return "", fmt.Errorf("vault: acquire subject vault: %w", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("vault: marshal payload: %w", err)
	}
	ct, err := sv.EncryptWithAAD(raw, itemAAD(orgID, secretID, version))
	if err != nil {
		return "", fmt.Errorf("vault: seal payload: %w", err)
	}
	return ct, nil
}

func (p *Plugin) openPayload(orgID uint, secretID string, version int, ciphertext string) (map[string]any, error) {
	c, err := p.requireCrypto()
	if err != nil {
		return nil, err
	}
	if ciphertext == "" {
		return map[string]any{}, nil
	}
	sv, err := c.SubjectVault(orgID, itemSubject(secretID))
	if err != nil {
		return nil, fmt.Errorf("vault: acquire subject vault: %w", err)
	}
	plain, err := sv.DecryptWithAAD(ciphertext, itemAAD(orgID, secretID, version))
	if err != nil {
		return nil, fmt.Errorf("vault: decrypt payload: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, fmt.Errorf("vault: decode payload: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (p *Plugin) shredItemKey(orgID uint, secretID string) error {
	c, err := p.requireCrypto()
	if err != nil {
		return err
	}
	return c.ShredSubjectKey(orgID, itemSubject(secretID))
}

// HasEncryptedData reports whether any vault version ciphertext exists, so boot
// stops when a fresh envelope DEK would orphan existing secrets.
func (p *Plugin) HasEncryptedData() (bool, error) {
	if p.db == nil {
		return false, nil
	}
	if !p.db.Migrator().HasTable(&VaultItemVersion{}) {
		return false, nil
	}
	var count int64
	if err := p.db.Model(&VaultItemVersion{}).Where("ciphertext <> ''").Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Validate enforces fail-closed startup invariants: the cryptographic vault must
// be present, and every existing encrypted item must still have its wrapped subject DEK.
func (p *Plugin) Validate(ctx context.Context, host plugin.Host) error {
	if host != nil {
		p.host = host
	}
	c, err := p.requireCrypto()
	if err != nil {
		return err
	}
	if p.db == nil || !p.db.Migrator().HasTable(&VaultItem{}) {
		return nil
	}
	var items []VaultItem
	if err := p.db.WithContext(ctx).Where("current_version > 0").Find(&items).Error; err != nil {
		return fmt.Errorf("vault: validate items: %w", err)
	}
	for _, it := range items {
		if !c.HasSubjectKey(it.OrgID, itemSubject(it.SecretID)) {
			return fmt.Errorf("vault: subject DEK missing for item %d (%s); restore backup before booting", it.ID, it.SecretID)
		}
	}
	return p.MigrateLegacyCoreSecrets(ctx)
}
