package llmgateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrNotFound reports a provider this workspace does not have.
	ErrNotFound = errors.New("llm gateway: provider not found")
	// ErrNotConfigured reports a workspace with no usable provider.
	ErrNotConfigured = errors.New("llm gateway: no LLM provider is configured for this workspace")
	// ErrInvalidProvider reports a provider row that cannot be built.
	ErrInvalidProvider = errors.New("llm gateway: invalid provider")
)

// ProviderInput is the writable shape of a provider. An empty APIKey keeps the
// stored key, so a client can rename a provider without re-entering the secret.
type ProviderInput struct {
	Name       string
	Backend    string
	APIKey     string
	BaseURL    string
	Model      string
	CheapModel string
}

// Store is the tenant-scoped provider and price registry. Every method takes
// the org it acts for; the handle refuses org 0, so an unattributed call can
// never read another workspace's rows.
type Store struct {
	db *gorm.DB
}

// NewStore builds a store over the shared database handle.
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

// ListProviders returns the workspace's providers, default first.
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

// GetProvider returns one provider of the workspace.
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

// CreateProvider inserts a provider. The first provider of a workspace becomes
// the default, so a workspace with one provider needs no second step.
func (s *Store) CreateProvider(ctx context.Context, orgID uint, in ProviderInput, seal func(string) (string, error)) (*Provider, error) {
	if err := validateInput(in); err != nil {
		return nil, err
	}
	tdb, err := s.tenant(orgID)
	if err != nil {
		return nil, err
	}
	sealed, err := sealKey(in.APIKey, seal)
	if err != nil {
		return nil, err
	}
	count, err := s.count(ctx, orgID)
	if err != nil {
		return nil, err
	}
	row := Provider{
		OrgID:      orgID,
		Name:       strings.TrimSpace(in.Name),
		Backend:    strings.TrimSpace(in.Backend),
		APIKeyEncr: sealed,
		BaseURL:    strings.TrimSpace(in.BaseURL),
		Model:      strings.TrimSpace(in.Model),
		CheapModel: strings.TrimSpace(in.CheapModel),
		IsDefault:  count == 0,
	}
	if err := tdb.Scoped(&Provider{}).WithContext(ctx).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("llm gateway: create provider: %w", err)
	}
	if count == 0 {
		if err := s.SetDefault(ctx, orgID, row.ID); err != nil {
			return nil, err
		}
	}
	return &row, nil
}

// UpdateProvider replaces the writable fields. An empty APIKey keeps the key.
func (s *Store) UpdateProvider(ctx context.Context, orgID, id uint, in ProviderInput, seal func(string) (string, error)) (*Provider, error) {
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
	updates := map[string]any{
		"name":        strings.TrimSpace(in.Name),
		"backend":     strings.TrimSpace(in.Backend),
		"base_url":    strings.TrimSpace(in.BaseURL),
		"model":       strings.TrimSpace(in.Model),
		"cheap_model": strings.TrimSpace(in.CheapModel),
	}
	if key := strings.TrimSpace(in.APIKey); key != "" {
		sealed, err := sealKey(key, seal)
		if err != nil {
			return nil, err
		}
		updates["api_key_encr"] = sealed
	}
	if err := tdb.Scoped(&Provider{}).WithContext(ctx).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("llm gateway: update provider: %w", err)
	}
	return s.GetProvider(ctx, orgID, id)
}

// DeleteProvider removes a provider. Deleting the default leaves the workspace
// without a default; ActiveProvider then fails closed until one is chosen.
func (s *Store) DeleteProvider(ctx context.Context, orgID, id uint) error {
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
	return nil
}

// SetDefault makes one provider the workspace default, clearing the flag on the
// others in the same transaction so two rows can never both be default.
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

// ActiveProvider returns the provider that serves the workspace: the default
// when one is set, otherwise the single configured provider. Two providers and
// no default is an error rather than a coin flip.
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

// ListPrices returns every priced model of the workspace.
func (s *Store) ListPrices(ctx context.Context, orgID uint) ([]ModelPrice, error) {
	tdb, err := s.tenant(orgID)
	if err != nil {
		return nil, err
	}
	var rows []ModelPrice
	if err := tdb.Scoped(&ModelPrice{}).WithContext(ctx).Order("model asc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("llm gateway: list prices: %w", err)
	}
	return rows, nil
}

// SetPrice inserts or replaces the price of one model.
func (s *Store) SetPrice(ctx context.Context, orgID uint, price ModelPrice) error {
	model := strings.TrimSpace(price.Model)
	if model == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidProvider)
	}
	if _, err := s.tenant(orgID); err != nil {
		return err
	}
	if price.Input < 0 || price.Output < 0 || price.Reasoning < 0 {
		return fmt.Errorf("%w: a price cannot be negative", ErrInvalidProvider)
	}
	row := ModelPrice{OrgID: orgID, Model: model, Input: price.Input, Output: price.Output, Reasoning: price.Reasoning}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "model"}},
		DoUpdates: clause.AssignmentColumns([]string{"input", "output", "reasoning", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("llm gateway: set price: %w", err)
	}
	return nil
}

// DeletePrice removes the price of one model.
func (s *Store) DeletePrice(ctx context.Context, orgID uint, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidProvider)
	}
	tdb, err := s.tenant(orgID)
	if err != nil {
		return err
	}
	return tdb.Scoped(&ModelPrice{}).WithContext(ctx).Where("model = ?", model).Delete(&ModelPrice{}).Error
}

// Price returns the price of one model, or plugin.ErrModelUnpriced.
func (s *Store) Price(ctx context.Context, orgID uint, model string) (plugin.ModelPrice, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return plugin.ModelPrice{}, plugin.ErrModelUnpriced
	}
	tdb, err := s.tenant(orgID)
	if err != nil {
		return plugin.ModelPrice{}, err
	}
	var row ModelPrice
	err = tdb.Scoped(&ModelPrice{}).WithContext(ctx).Where("model = ?", model).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return plugin.ModelPrice{}, plugin.ErrModelUnpriced
	}
	if err != nil {
		return plugin.ModelPrice{}, fmt.Errorf("llm gateway: read price: %w", err)
	}
	return plugin.ModelPrice{Input: row.Input, Output: row.Output, Reasoning: row.Reasoning}, nil
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

func sealKey(key string, seal func(string) (string, error)) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}
	if seal == nil {
		return "", fmt.Errorf("%w: secret storage is unavailable", ErrInvalidProvider)
	}
	sealed, err := seal(key)
	if err != nil {
		return "", fmt.Errorf("%w: seal API key: %v", ErrInvalidProvider, err)
	}
	return sealed, nil
}
