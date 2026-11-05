package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/octarq-org/octarq/server/idempotency"
	"github.com/octarq-org/octarq/server/internal/db"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
)

// prepareMigrationAndCipher performs startup preflights, refuses a database
// migrated by a newer binary, backs the database up before an upgrade,
// auto-migrates all core and plugin models together in a single pass, records
// the schema version, and upgrades the cipher to envelope mode.
func (a *App) prepareMigrationAndCipher() error {
	if err := preflightNameCollisions(a.plugins); err != nil {
		return err
	}
	if err := preflightDependencies(a.plugins); err != nil {
		return err
	}
	if err := preflightTableCollisions(a.gdb.NamingStrategy, a.plugins); err != nil {
		return err
	}

	upgrade, err := db.CheckSchemaVersion(a.gdb)
	if err != nil {
		return err
	}
	// A schema that already exists and is about to change is backed up first:
	// AutoMigrate has no downgrade path, so the copy is the way back.
	if upgrade && a.gdb.Migrator().HasTable(&models.User{}) {
		if err := a.backupBeforeMigration(); err != nil {
			return err
		}
	}

	// The webhook delivery log and the idempotency ledger are core
	// infrastructure that no plugin owns, so they join the same single pass.
	extra := append(eventbus.Models(), idempotency.Models()...)
	for _, p := range a.plugins {
		extra = append(extra, p.Models()...)
	}
	if err := db.Migrate(a.gdb, extra...); err != nil {
		return err
	}
	if err := db.RecordSchemaVersion(a.gdb, db.SchemaVersion); err != nil {
		return err
	}
	return a.cipher.EnableEnvelope(settingsStore{a.gdb})
}

// backupBeforeMigration writes a database snapshot before an upgrade. A failed
// backup stops the boot: migrating without a copy leaves nothing to return to,
// and AutoMigrate never reverses itself.
func (a *App) backupBeforeMigration() error {
	dir := filepath.Join(filepath.Dir(a.cfg.StorageDir), "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create the backup directory %q: %w", dir, err)
	}
	name := "octarq-premigrate-" + time.Now().UTC().Format("20060102-150405") + ".db"
	if a.cfg.DBDriver == "postgres" || a.cfg.DBDriver == "mysql" {
		name = name[:len(name)-3] + ".sql"
	}
	path := filepath.Join(dir, name)
	if err := db.Backup(a.cfg, path); err != nil {
		return fmt.Errorf("back up the database before the schema upgrade: %w", err)
	}
	slog.Info("pre-migration backup written", "path", path, "driver", a.cfg.DBDriver)
	return nil
}
