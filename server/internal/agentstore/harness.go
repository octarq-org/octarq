// Package agentstore persists the published agent harness of a workspace.
//
// The harness itself is a pure micro-kernel with no storage of its own; the
// store is what turns a harness spec into something an operator can publish,
// list and roll back. Everything here is tenant-scoped: a row belongs to one
// workspace and is never visible to another.
package agentstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a workspace has no such harness version.
var ErrNotFound = errors.New("agentstore: harness spec not found")

// HarnessStore reads and writes the published harness versions of a
// workspace.
type HarnessStore struct {
	db *gorm.DB
}

// NewHarnessStore constructs a store over the given database.
func NewHarnessStore(db *gorm.DB) *HarnessStore {
	return &HarnessStore{db: db}
}

// Publish validates a spec, stores it as a new version and makes it the
// active one. The previous version is archived, never deleted: a rollback
// needs it.
//
// An invalid spec is refused as a whole — nothing is written.
func (s *HarnessStore) Publish(ctx context.Context, orgID uint, spec harness.HarnessSpec, actorID uint) (*models.AgentHarnessSpec, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: harness store is not configured")
	}
	if err := harness.Validate(spec); err != nil {
		return nil, err
	}

	canonical := spec.CanonicalJSON()
	fingerprint := spec.Fingerprint()

	now := time.Now().UTC()
	row := &models.AgentHarnessSpec{
		OrgID:       orgID,
		Name:        spec.Name,
		Version:     0,
		SpecJSON:    canonical,
		Fingerprint: fingerprint,
		Status:      models.AgentHarnessStatusArchived,
		CreatedBy:   actorID,
		CreatedAt:   now,
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&models.AgentHarnessSpec{}).
			Where("owner_id = ? AND name = ?", orgID, spec.Name).
			Select("COALESCE(MAX(version), 0)").
			Scan(&maxVersion).Error; err != nil {
			return err
		}
		row.Version = maxVersion + 1

		if err := tx.Model(&models.AgentHarnessSpec{}).
			Where("owner_id = ? AND name = ? AND status = ?", orgID, spec.Name, models.AgentHarnessStatusPublished).
			Update("status", models.AgentHarnessStatusArchived).Error; err != nil {
			return err
		}

		row.Status = models.AgentHarnessStatusPublished
		row.PublishedAt = &now
		return tx.Create(row).Error
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Rollback makes an archived version the active one again.
func (s *HarnessStore) Rollback(ctx context.Context, orgID, id, actorID uint) (*models.AgentHarnessSpec, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: harness store is not configured")
	}

	var target models.AgentHarnessSpec
	if err := s.db.WithContext(ctx).Where("id = ? AND owner_id = ?", id, orgID).First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.AgentHarnessSpec{}).
			Where("owner_id = ? AND name = ? AND status = ?", orgID, target.Name, models.AgentHarnessStatusPublished).
			Update("status", models.AgentHarnessStatusArchived).Error; err != nil {
			return err
		}
		return tx.Model(&models.AgentHarnessSpec{}).
			Where("id = ? AND owner_id = ?", id, orgID).
			Updates(map[string]any{
				"status":       models.AgentHarnessStatusPublished,
				"published_at": &now,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&target).Error; err != nil {
		return nil, err
	}
	return &target, nil
}

// Active returns the published version of the workspace, or ErrNotFound when
// the workspace has not published one.
func (s *HarnessStore) Active(ctx context.Context, orgID uint) (*models.AgentHarnessSpec, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotFound
	}
	var row models.AgentHarnessSpec
	err := s.db.WithContext(ctx).
		Where("owner_id = ? AND status = ?", orgID, models.AgentHarnessStatusPublished).
		Order("version DESC").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// Get returns one version of the workspace.
func (s *HarnessStore) Get(ctx context.Context, orgID, id uint) (*models.AgentHarnessSpec, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotFound
	}
	var row models.AgentHarnessSpec
	if err := s.db.WithContext(ctx).Where("id = ? AND owner_id = ?", id, orgID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// List returns every version of the workspace, newest first.
func (s *HarnessStore) List(ctx context.Context, orgID uint) ([]models.AgentHarnessSpec, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	var rows []models.AgentHarnessSpec
	if err := s.db.WithContext(ctx).
		Where("owner_id = ?", orgID).
		Order("version DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Spec resolves the harness spec a workspace runs with: the published one, or
// the built-in default when the workspace has not published anything.
//
// A published spec that cannot be decoded is an error: it is never silently
// replaced by the default, because that would hide a broken configuration.
func (s *HarnessStore) Spec(ctx context.Context, orgID uint) (harness.HarnessSpec, error) {
	row, err := s.Active(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return harness.DefaultHarnessSpec(), nil
		}
		return harness.HarnessSpec{}, err
	}
	spec, err := harness.DecodeSpec([]byte(row.SpecJSON))
	if err != nil {
		return harness.HarnessSpec{}, fmt.Errorf("published harness spec %d is invalid: %w", row.ID, err)
	}
	return spec, nil
}
