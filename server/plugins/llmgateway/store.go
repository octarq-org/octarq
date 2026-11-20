package llmgateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

var (
	ErrNotFound = errors.New("llm gateway: provider not found")

	ErrNotConfigured = errors.New("llm gateway: no LLM provider is configured for this workspace")

	ErrInvalidProvider = errors.New("llm gateway: invalid provider")
)

type ProviderInput struct {
	Name       string
	Backend    string
	APIKey     string
	BaseURL    string
	Model      string
	CheapModel string
}

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) tenant(orgID uint) (*tenantsql.TenantDB, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("%w: database unavailable", ErrInvalidProvider)
	}
	tdb, err := tenantsql.NewTenantDB(s.db, orgID, tenantsql.WithTenantColumn("org_id"))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	return tdb, nil
}

func (s *Store) ListProviders(ctx context.Context, orgID uint) ([]Provider, error) {
	tdb, err := s.tenant(orgID)
	if err != nil {
		return nil, err
	}
	var rows []Provider
	err = tdb.Scoped(&Provider{}).WithContext(ctx).
		Order("is_default desc, id asc").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("llm gateway: list providers: %w", err)
	}
	return rows, nil
}

func (s *Store) GetProvider(ctx context.Context, orgID, id uint) (*Provider, error) {
	tdb, err := s.tenant(orgID)
	if err != nil {
		return nil, err
	}
	var row Provider
	err = tdb.Scoped(&Provider{}).WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("llm gateway: get provider: %w", err)
	}
	return &row, nil
}

func (s *Store) CreateProvider(ctx context.Context, orgID uint, in ProviderInput, vs plugin.VaultService) (*Provider, error) {
	if err := validateInput(in); err != nil {
		return nil, err
	}
	tdb, err := s.tenant(orgID)
	if err != nil {
		return nil, err
	}
	var vaultItemID uint
	if key := strings.TrimSpace(in.APIKey); key != "" {
		if vs == nil {
			return nil, fmt.Errorf("%w: secret storage is unavailable", ErrInvalidProvider)
		}
		meta, err := vs.PutSecret(ctx, llmVaultPutRequest(orgID, in, key))
		if err != nil {
			return nil, fmt.Errorf("%w: seal API key: %v", ErrInvalidProvider, err)
		}
		vaultItemID = meta.ID
	}
	count, err := s.count(ctx, orgID)
	if err != nil {
		if vaultItemID > 0 && vs != nil {
			_ = vs.DeleteSecret(ctx, orgID, vaultItemID)
		}
		return nil, err
	}
	row := Provider{
		OrgID:       orgID,
		Name:        strings.TrimSpace(in.Name),
		Backend:     strings.TrimSpace(in.Backend),
		VaultItemID: vaultItemID,
		BaseURL:     strings.TrimSpace(in.BaseURL),
		Model:       strings.TrimSpace(in.Model),
		CheapModel:  strings.TrimSpace(in.CheapModel),
		IsDefault:   count == 0,
	}
	if err := tdb.Scoped(&Provider{}).WithContext(ctx).Create(&row).Error; err != nil {
		if vaultItemID > 0 && vs != nil {
			_ = vs.DeleteSecret(ctx, orgID, vaultItemID)
		}
		return nil, fmt.Errorf("llm gateway: create provider: %w", err)
	}
	if count == 0 {
		if err := s.SetDefault(ctx, orgID, row.ID); err != nil {
			return nil, err
		}
	}
	return &row, nil
}

func (s *Store) UpdateProvider(ctx context.Context, orgID, id uint, in ProviderInput, vs plugin.VaultService) (*Provider, error) {
	if err := validateInput(in); err != nil {
		return nil, err
	}
	row, err := s.GetProvider(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	tdb, err := s.tenant(orgID)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	updates := map[string]any{
		"name":        name,
		"backend":     strings.TrimSpace(in.Backend),
		"base_url":    strings.TrimSpace(in.BaseURL),
		"model":       strings.TrimSpace(in.Model),
		"cheap_model": strings.TrimSpace(in.CheapModel),
	}
	if key := strings.TrimSpace(in.APIKey); key != "" {
		if vs == nil {
			return nil, fmt.Errorf("%w: secret storage is unavailable", ErrInvalidProvider)
		}
		payload := llmVaultPayload(in, key)
		if row.VaultItemID > 0 {
			_, err := vs.UpdateSecret(ctx, orgID, row.VaultItemID, &name, nil, payload, nil, "llmgateway provider updated", 0)
			if errors.Is(err, plugin.ErrSecretNotFound) {
				meta, putErr := vs.PutSecret(ctx, llmVaultPutRequest(orgID, in, key))
				if putErr != nil {
					return nil, fmt.Errorf("%w: seal API key: %v", ErrInvalidProvider, putErr)
				}
				updates["vault_item_id"] = meta.ID
			} else if err != nil {
				return nil, fmt.Errorf("%w: seal API key: %v", ErrInvalidProvider, err)
			}
		} else {
			meta, err := vs.PutSecret(ctx, llmVaultPutRequest(orgID, in, key))
			if err != nil {
				return nil, fmt.Errorf("%w: seal API key: %v", ErrInvalidProvider, err)
			}
			updates["vault_item_id"] = meta.ID
		}
	}
	if err := tdb.Scoped(&Provider{}).WithContext(ctx).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("llm gateway: update provider: %w", err)
	}
	return s.GetProvider(ctx, orgID, id)
}

func (s *Store) DeleteProvider(ctx context.Context, orgID, id uint, vs plugin.VaultService) error {
	row, err := s.GetProvider(ctx, orgID, id)
	if err != nil {
		return err
	}
	tdb, err := s.tenant(orgID)
	if err != nil {
		return err
	}
	if err := tdb.Scoped(&Provider{}).WithContext(ctx).Where("id = ?", row.ID).Delete(&Provider{}).Error; err != nil {
		return fmt.Errorf("llm gateway: delete provider: %w", err)
	}
	if row.VaultItemID > 0 && vs != nil {
		if err := vs.DeleteSecret(ctx, orgID, row.VaultItemID); err != nil && !errors.Is(err, plugin.ErrSecretNotFound) {
			return fmt.Errorf("llm gateway: delete provider secret: %w", err)
		}
	}
	return nil
}

func (s *Store) SetDefault(ctx context.Context, orgID, id uint) error {
	if _, err := s.GetProvider(ctx, orgID, id); err != nil {
		return err
	}
	if _, err := s.tenant(orgID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Provider{}).Where("org_id = ?", orgID).
			Update("is_default", false).Error; err != nil {
			return err
		}
		res := tx.Model(&Provider{}).Where("org_id = ? AND id = ?", orgID, id).
			Update("is_default", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) ActiveProvider(ctx context.Context, orgID uint) (*Provider, error) {
	rows, err := s.ListProviders(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotConfigured
	}
	for i := range rows {
		if rows[i].IsDefault {
			return &rows[i], nil
		}
	}
	if len(rows) == 1 {
		return &rows[0], nil
	}
	return nil, fmt.Errorf("%w: choose a default provider", ErrNotConfigured)
}

func (s *Store) Price(ctx context.Context, orgID uint, model string) (plugin.ModelPrice, error) {
	c, err := pricingCatalog()
	if err != nil {
		return plugin.ModelPrice{}, plugin.ErrPricingUnavailable
	}
	return c.price(model)
}

func (s *Store) count(ctx context.Context, orgID uint) (int64, error) {
	tdb, err := s.tenant(orgID)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := tdb.Scoped(&Provider{}).WithContext(ctx).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("llm gateway: count providers: %w", err)
	}
	return n, nil
}

func validateInput(in ProviderInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidProvider)
	}
	if strings.TrimSpace(in.Backend) == "" {
		return fmt.Errorf("%w: backend is required", ErrInvalidProvider)
	}
	return nil
}

func llmVaultPayload(in ProviderInput, key string) map[string]any {
	payload := map[string]any{
		"apiKey": strings.TrimSpace(key),
	}
	if b := strings.TrimSpace(in.BaseURL); b != "" {
		payload["baseUrl"] = b
	}
	if m := strings.TrimSpace(in.Model); m != "" {
		payload["model"] = m
	}
	if cm := strings.TrimSpace(in.CheapModel); cm != "" {
		payload["cheapModel"] = cm
	}
	return payload
}

func llmVaultPutRequest(orgID uint, in ProviderInput, key string) plugin.PutSecretRequest {
	return plugin.PutSecretRequest{
		OrgID:          orgID,
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "integrations",
		ItemType:       "integration",
		ProviderType:   "llm",
		Provider:       strings.ToLower(strings.TrimSpace(in.Backend)),
		Name:           strings.TrimSpace(in.Name),
		Payload:        llmVaultPayload(in, key),
	}
}
