// Package db opens the configured database (SQLite or Postgres) through GORM
// and runs migrations. SQLite uses the pure-Go glebarez driver so the final
// binary needs no cgo.
package db

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"github.com/jungley/led/config"
	"github.com/jungley/led/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to the database and auto-migrates the schema.
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
	if err := gdb.AutoMigrate(models.AllModels()...); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
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
					OwnerID: models.SingleUserID,
					Name:    ld.Provider + " (Migrated)",
					Type:    ld.Provider,
					Config:  ld.Config,
				}
				gdb.Create(&acc)
			}
			gdb.Exec("UPDATE domains SET provider_account_id = ? WHERE id = ?", acc.ID, ld.ID)
		}
	}

	return gdb, nil
}
