// Package db opens the configured database (SQLite or Postgres) through GORM
// and runs migrations. SQLite uses the pure-Go glebarez driver so the final
// binary needs no cgo.
package db

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to the database WITHOUT migrating. Migration is deferred to
// Migrate so that plugin-contributed models (registered after Open) are
// migrated together with the core schema in a single pass — see the plugin
// package for why AutoMigrate must not run at open time.
func Open(cfg *config.Config) (*gorm.DB, error) {
	var dial gorm.Dialector
	switch cfg.DBDriver {
	case "sqlite":
		dial = sqlite.Open(cfg.DBDSN)
	case "postgres":
		dial = postgres.Open(cfg.DBDSN)
	default:
		return nil, fmt.Errorf("unsupported driver %q", cfg.DBDriver)
	}

	gdb, err := gorm.Open(dial, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return gdb, nil
}

// Migrate auto-migrates the core schema plus any extra (plugin) models, then
// runs one-off data migrations. Call this once, after every plugin has been
// registered, before serving traffic.
func Migrate(gdb *gorm.DB, extraModels ...any) error {
	// Drop legacy sessions table if it lacks the 'id' primary key column (handles SQLite migration limits)
	if gdb.Migrator().HasTable(&models.Session{}) {
		if !gdb.Migrator().HasColumn(&models.Session{}, "id") {
			_ = gdb.Migrator().DropTable(&models.Session{})
		}
	}

	all := append(models.AllModels(), extraModels...)
	if err := gdb.AutoMigrate(all...); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Data migration: move org-level settings from global settings to workspace_settings (for org 1)
	for _, key := range []string{"catch_all", "auto_wrap_links", "reserved_mailboxes"} {
		var s models.Setting
		if err := gdb.Where("key = ?", key).First(&s).Error; err == nil {
			var count int64
			gdb.Model(&models.WorkspaceSetting{}).Where("org_id = ? AND key = ?", 1, key).Count(&count)
			if count == 0 {
				gdb.Create(&models.WorkspaceSetting{OrgID: 1, Key: key, Value: s.Value})
			}
			gdb.Delete(&s)
		}
	}

	// Data migration: backfill User.IsInstanceAdmin for existing installs.
	// Instance admin used to be derived from "owner of org 1"; it is now a
	// stable per-user flag set at admin login (see api.bootstrapUserID). For a
	// pre-existing deployment whose admin may not log in again immediately, seed
	// the flag once for the current org-1 owner so it doesn't lose admin. Guard
	// on "no user already flagged" so this runs exactly once and a fresh install
	// (where the flag is set the proper way at first login) is never touched.
	{
		var flagged int64
		gdb.Model(&models.User{}).Where("is_instance_admin = ?", true).Count(&flagged)
		if flagged == 0 {
			var ownerID uint
			if err := gdb.Model(&models.OrgMember{}).
				Where("org_id = ? AND role = ?", 1, "owner").
				Order("user_id ASC").
				Limit(1).
				Pluck("user_id", &ownerID).Error; err == nil && ownerID != 0 {
				gdb.Model(&models.User{}).Where("id = ?", ownerID).Update("is_instance_admin", true)
			}
		}
	}

	return nil
}
