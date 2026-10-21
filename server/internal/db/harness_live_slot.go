package db

import (
	"errors"
	"fmt"
	"log"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

// A workspace runs one harness. The store's transaction keeps that true
// inside one process; the database keeps it true when two requests race.
//
// The database needs a key to enforce it, and a status column cannot be one:
// "published" is a value many rows may hold. The key is a nullable column
// that carries 1 on the live row and NULL on every other row, inside the
// unique index on (owner_id, live_slot). NULLs are distinct on every
// supported database, so any number of drafts may carry NULL while only one
// row may carry 1.
//
// The column is written in the same statement as the status and is never read
// to decide which version is live. The status is the truth; the column is the
// constraint.
const (
	// harnessLiveSlotColumn is the nullable column the unique index is built
	// on.
	harnessLiveSlotColumn = "live_slot"
	// harnessLiveSlotValue is the value the live version carries.
	harnessLiveSlotValue = 1
)

// repairHarnessLiveSlots gives every workspace exactly one live version.
//
// It runs BEFORE the schema changes, because the unique index that enforces
// the rule cannot be created while two rows of one workspace still claim to
// be live. An install that ran a double activation before this rule existed
// can hold two published rows for one workspace: the newest keeps the
// workspace live, and the older one is archived, because a workspace runs one
// harness and the newer version is the one an operator last chose.
//
// The repair is idempotent: once the index exists, a workspace cannot hold
// two published rows, so the repair finds nothing to do.
func repairHarnessLiveSlots(gdb *gorm.DB) error {
	if gdb == nil {
		return errors.New("migrate: no database")
	}
	if !gdb.Migrator().HasTable(&models.AgentHarnessSpec{}) {
		return nil
	}

	type publishedRow struct {
		ID      uint
		OwnerID uint
		Version int
	}
	var rows []publishedRow
	if err := gdb.Table("agent_harness_specs").
		Select("id, owner_id, version").
		Where("status = ?", models.AgentHarnessStatusPublished).
		Order("owner_id ASC, version DESC, id DESC").
		Scan(&rows).Error; err != nil {
		return fmt.Errorf("migrate: read the published harness versions: %w", err)
	}

	seen := make(map[uint]bool, len(rows))
	var duplicates []uint
	for _, r := range rows {
		if !seen[r.OwnerID] {
			seen[r.OwnerID] = true
			continue
		}
		duplicates = append(duplicates, r.ID)
	}
	if len(duplicates) == 0 {
		return nil
	}

	log.Printf("[migrate] %d harness version(s) were published twice; archiving the older ones", len(duplicates))
	if err := gdb.Model(&models.AgentHarnessSpec{}).
		Where("id IN ?", duplicates).
		Updates(map[string]any{
			"status":       models.AgentHarnessStatusArchived,
			"published_at": nil,
		}).Error; err != nil {
		return fmt.Errorf("migrate: archive the duplicate harness versions: %w", err)
	}
	return nil
}

// markHarnessLiveSlots writes the live slot after the column exists.
//
// It runs AFTER the schema changes, because the column is created there. The
// slot follows the status: the live version holds it, every other row
// releases it.
func markHarnessLiveSlots(gdb *gorm.DB) error {
	if gdb == nil {
		return errors.New("migrate: no database")
	}
	m := gdb.Migrator()
	if !m.HasTable(&models.AgentHarnessSpec{}) {
		return nil
	}
	if !m.HasColumn(&models.AgentHarnessSpec{}, harnessLiveSlotColumn) {
		return nil
	}

	if err := gdb.Model(&models.AgentHarnessSpec{}).
		Where("status = ?", models.AgentHarnessStatusPublished).
		Update(harnessLiveSlotColumn, harnessLiveSlotValue).Error; err != nil {
		return fmt.Errorf("migrate: mark the live harness version: %w", err)
	}
	if err := gdb.Model(&models.AgentHarnessSpec{}).
		Where("status <> ? OR status IS NULL", models.AgentHarnessStatusPublished).
		Update(harnessLiveSlotColumn, nil).Error; err != nil {
		return fmt.Errorf("migrate: release the harness live slot: %w", err)
	}
	return nil
}
