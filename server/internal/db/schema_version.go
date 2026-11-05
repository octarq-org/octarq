package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SchemaVersion is the schema this binary writes. Bump it in the same change
// that alters a model, so a binary older than the database can tell that it is
// older and refuse to start.
const SchemaVersion int64 = 1

// ErrSchemaNewer reports a database migrated by a newer binary.
var ErrSchemaNewer = errors.New("the database was migrated by a newer octarq release; start that release again, or restore the backup taken before the upgrade")

// SchemaVersionOf returns the version the database was last migrated to. A
// database that predates the version table returns 0.
func SchemaVersionOf(gdb *gorm.DB) (int64, error) {
	if !gdb.Migrator().HasTable(&models.SchemaMigration{}) {
		return 0, nil
	}
	var row models.SchemaMigration
	err := gdb.Order("version DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the schema version: %w", err)
	}
	return row.Version, nil
}

// RecordSchemaVersion marks the database as migrated to version.
func RecordSchemaVersion(gdb *gorm.DB, version int64) error {
	row := models.SchemaMigration{Version: version, AppliedAt: time.Now().UTC()}
	if err := gdb.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("record the schema version: %w", err)
	}
	return nil
}

// CheckSchemaVersion refuses a database migrated by a newer binary, and reports
// whether this boot upgrades the schema (recorded < SchemaVersion).
func CheckSchemaVersion(gdb *gorm.DB) (upgrade bool, err error) {
	recorded, err := SchemaVersionOf(gdb)
	if err != nil {
		return false, err
	}
	if recorded > SchemaVersion {
		return false, fmt.Errorf("%w (database version %d, this binary version %d)", ErrSchemaNewer, recorded, SchemaVersion)
	}
	return recorded < SchemaVersion, nil
}
