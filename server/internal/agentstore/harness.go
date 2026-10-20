// Package agentstore persists the published agent harness of a workspace.
//
// The harness itself is a pure micro-kernel with no storage of its own; the
// store is what turns a harness spec into something an operator can stage,
// publish, roll back and learn from. Everything here is tenant-scoped: a row
// belongs to one workspace and is never visible to another.
package agentstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a workspace has no such harness version.
var ErrNotFound = errors.New("agentstore: harness spec not found")

// ErrInvalidState is returned when an operation does not fit the version's
// current state — discarding a live version, for instance.
var ErrInvalidState = errors.New("agentstore: harness version is not in a state that allows this")

// HarnessStore reads and writes the harness versions of a workspace.
type HarnessStore struct {
	db *gorm.DB
}

// NewHarnessStore constructs a store over the given database.
func NewHarnessStore(db *gorm.DB) *HarnessStore {
	return &HarnessStore{db: db}
}

// Stage stores a spec as a **draft**: a version that is not live. Drafts are
// what a human reviews before activating, and what an imported lab proposal
// turns into.
//
// An invalid spec is refused as a whole — nothing is written.
func (s *HarnessStore) Stage(ctx context.Context, orgID uint, spec harness.HarnessSpec, actorID uint) (*models.AgentHarnessSpec, error) {
	return s.stage(ctx, orgID, spec, actorID, nil)
}

// ImportProposal stores a lab proposal as a draft, keeping the evidence that
// came with it. The proposal's spec is validated exactly like a hand-written
// one: a proposal that fails validation is refused, not "best effort".
func (s *HarnessStore) ImportProposal(ctx context.Context, orgID uint, spec harness.HarnessSpec, evidence json.RawMessage, actorID uint) (*models.AgentHarnessSpec, error) {
	return s.stage(ctx, orgID, spec, actorID, evidence)
}

func (s *HarnessStore) stage(ctx context.Context, orgID uint, spec harness.HarnessSpec, actorID uint, evidence json.RawMessage) (*models.AgentHarnessSpec, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: harness store is not configured")
	}
	if err := harness.Validate(spec); err != nil {
		return nil, err
	}

	evidenceText := ""
	if len(evidence) > 0 {
		evidenceText = string(evidence)
	}

	row := &models.AgentHarnessSpec{
		OrgID:       orgID,
		Name:        spec.Name,
		Version:     0,
		SpecJSON:    spec.CanonicalJSON(),
		Fingerprint: spec.Fingerprint(),
		Status:      models.AgentHarnessStatusDraft,
		Evidence:    evidenceText,
		CreatedBy:   actorID,
		CreatedAt:   time.Now().UTC(),
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&models.AgentHarnessSpec{}).
			Where("owner_id = ?", orgID).
			Select("COALESCE(MAX(version), 0)").
			Scan(&maxVersion).Error; err != nil {
			return err
		}
		row.Version = maxVersion + 1
		return tx.Create(row).Error
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Activate makes a version the live one.
//
// Exactly one version per workspace is live. Activating archives whatever was
// live before — across every name, because the workspace runs one harness.
// A draft and an archived version can both be activated; activating an
// archived version is what a rollback is.
func (s *HarnessStore) Activate(ctx context.Context, orgID, id, actorID uint) (*models.AgentHarnessSpec, error) {
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
	if target.Status == models.AgentHarnessStatusDiscarded {
		return nil, fmt.Errorf("%w: a discarded version cannot be activated", ErrInvalidState)
	}

	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Archive every live version of the workspace, whatever its name.
		if err := tx.Model(&models.AgentHarnessSpec{}).
			Where("owner_id = ? AND status = ?", orgID, models.AgentHarnessStatusPublished).
			Update("status", models.AgentHarnessStatusArchived).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.AgentHarnessSpec{}).
			Where("id = ? AND owner_id = ?", id, orgID).
			Updates(map[string]any{
				"status":       models.AgentHarnessStatusPublished,
				"published_at": &now,
			}).Error; err != nil {
			return err
		}

		// A lesson that motivated this version has now been acted on.
		return tx.Model(&models.AgentHarnessLesson{}).
			Where("owner_id = ? AND spec_id = ? AND status = ?", orgID, id, models.AgentHarnessLessonSuggested).
			Update("status", models.AgentHarnessLessonAdopted).Error
	})
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&target).Error; err != nil {
		return nil, err
	}
	return &target, nil
}

// Discard throws a draft away. A version that was live once is kept: it is
// part of the workspace's history, and a rollback may still need it.
func (s *HarnessStore) Discard(ctx context.Context, orgID, id uint) (*models.AgentHarnessSpec, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotFound
	}

	var target models.AgentHarnessSpec
	if err := s.db.WithContext(ctx).Where("id = ? AND owner_id = ?", id, orgID).First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if target.Status != models.AgentHarnessStatusDraft {
		return nil, fmt.Errorf("%w: only a draft can be discarded", ErrInvalidState)
	}

	if err := s.db.WithContext(ctx).
		Model(&models.AgentHarnessSpec{}).
		Where("id = ? AND owner_id = ?", id, orgID).
		Update("status", models.AgentHarnessStatusDiscarded).Error; err != nil {
		return nil, err
	}

	// A lesson that was only a suggestion dies with the draft it motivated.
	if err := s.db.WithContext(ctx).
		Model(&models.AgentHarnessLesson{}).
		Where("owner_id = ? AND spec_id = ? AND status = ?", orgID, id, models.AgentHarnessLessonSuggested).
		Update("status", models.AgentHarnessLessonDismissed).Error; err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&target).Error; err != nil {
		return nil, err
	}
	return &target, nil
}

// Active returns the live version of the workspace, or ErrNotFound when the
// workspace has never activated one.
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

// Spec resolves the harness spec a workspace runs with: the live one, or the
// built-in default when the workspace has never activated a version.
//
// A live spec that cannot be decoded is an error: it is never silently
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

// CreateLesson records something the workspace learned about its agent.
func (s *HarnessStore) CreateLesson(ctx context.Context, orgID uint, scope, note string, evidence json.RawMessage, specID, actorID uint) (*models.AgentHarnessLesson, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("agentstore: harness store is not configured")
	}
	if scope == "" {
		return nil, errors.New("agentstore: a lesson needs a scope")
	}
	if note == "" {
		return nil, errors.New("agentstore: a lesson needs a note")
	}

	evidenceText := ""
	if len(evidence) > 0 {
		evidenceText = string(evidence)
	}

	row := &models.AgentHarnessLesson{
		OrgID:     orgID,
		Scope:     scope,
		Note:      note,
		Evidence:  evidenceText,
		Status:    models.AgentHarnessLessonSuggested,
		SpecID:    specID,
		CreatedBy: actorID,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// ListLessons returns the lessons of the workspace, newest first.
func (s *HarnessStore) ListLessons(ctx context.Context, orgID uint) ([]models.AgentHarnessLesson, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	var rows []models.AgentHarnessLesson
	if err := s.db.WithContext(ctx).
		Where("owner_id = ?", orgID).
		Order("created_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// DismissLesson marks a lesson as rejected. The lesson stays visible: what
// was considered and turned down is part of the record.
func (s *HarnessStore) DismissLesson(ctx context.Context, orgID, id uint) (*models.AgentHarnessLesson, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotFound
	}

	var target models.AgentHarnessLesson
	if err := s.db.WithContext(ctx).Where("id = ? AND owner_id = ?", id, orgID).First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if err := s.db.WithContext(ctx).
		Model(&models.AgentHarnessLesson{}).
		Where("id = ? AND owner_id = ?", id, orgID).
		Update("status", models.AgentHarnessLessonDismissed).Error; err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&target).Error; err != nil {
		return nil, err
	}
	return &target, nil
}
