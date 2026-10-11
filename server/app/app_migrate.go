package app

import (
	"github.com/octarq-org/octarq/server/idempotency"
	"github.com/octarq-org/octarq/server/internal/db"
	"github.com/octarq-org/octarq/server/internal/eventbus"
)

// prepareMigrationAndCipher performs startup preflights, auto-migrates all core
// and plugin models together in a single pass, and upgrades the cipher to envelope mode.
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
	// The webhook delivery log and the idempotency ledger are core
	// infrastructure that no plugin owns, so they join the same single pass.
	extra := append(eventbus.Models(), idempotency.Models()...)
	for _, p := range a.plugins {
		extra = append(extra, p.Models()...)
	}
	if err := db.Migrate(a.gdb, extra...); err != nil {
		return err
	}
	return a.cipher.EnableEnvelope(settingsStore{a.gdb})
}
