// Package db opens the configured database (SQLite or Postgres) through GORM
// and runs migrations. SQLite uses the pure-Go glebarez driver so the final
// binary needs no cgo.
package db

import (
	"fmt"

	"github.com/octarq-org/led/config"
	"github.com/octarq-org/led/internal/models"
	"github.com/glebarez/sqlite"
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

	// Data migration: move legacy domain.provider / config to ProviderAccount
	if gdb.Migrator().HasColumn(&models.Domain{}, "provider") && gdb.Migrator().HasColumn(&models.Domain{}, "config") {
		var legacyDomains []struct {
			ID       uint
			Provider string
			Config   string
		}
		// Read domains that haven't been migrated yet
		gdb.Raw("SELECT id, provider, config FROM domains WHERE provider_account_id = 0 OR provider_account_id IS NULL").Scan(&legacyDomains)
		for _, ld := range legacyDomains {
			if ld.Provider == "" {
				continue
			}
			var acc models.ProviderAccount
			// Group by identical config to avoid duplicating the same account
			if err := gdb.Where("config = ?", ld.Config).First(&acc).Error; err != nil {
				acc = models.ProviderAccount{
					OrgID:  models.SingleUserID,
					Name:   ld.Provider + " (Migrated)",
					Type:   ld.Provider,
					Config: ld.Config,
				}
				gdb.Create(&acc)
			}
			gdb.Exec("UPDATE domains SET provider_account_id = ? WHERE id = ?", acc.ID, ld.ID)
		}
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

	return nil
}

