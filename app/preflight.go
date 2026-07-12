package app

import (
	"fmt"
	"sync"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm/schema"
)

// preflightTableCollisions guards the single delayed AutoMigrate pass (core
// models + every plugin's Models(), see Run) against two distinct struct types
// silently mapping to the same table: AutoMigrate would apply both definitions
// in registration order and the later one would mutate the earlier one's schema.
//
// Table names are resolved the same way GORM's migrator resolves them — a
// TableName() string override wins, otherwise namer derives the name from the
// struct name — so the check sees exactly what AutoMigrate will see.
//
// One collision shape is deliberately allowed: a plugin mirroring an EXISTING
// core table with a local struct (e.g. `func (Setting) TableName() string {
// return "settings" }`) is the documented convention for plugins that read or
// extend core tables without importing internal/models, and must not fail
// startup. What fails is genuine ownership ambiguity: two different struct
// types from plugins (two plugins, or one plugin declaring both) claiming the
// same non-core table. Declaring the same struct type twice is harmless and
// tolerated (the pass is idempotent per type).
func preflightTableCollisions(namer schema.Namer, plugins []plugin.Plugin) error {
	if namer == nil {
		namer = schema.NamingStrategy{}
	}
	cache := &sync.Map{}
	tableOf := func(model any) (string, string, error) {
		s, err := schema.Parse(model, cache, namer)
		if err != nil {
			return "", "", err
		}
		return s.Table, s.ModelType.String(), nil
	}

	// Tables the core owns: plugin mirrors of these are allowed by convention.
	coreTables := make(map[string]bool)
	for _, m := range models.AllModels() {
		table, _, err := tableOf(m)
		if err != nil {
			return fmt.Errorf("preflight: parse core model %T: %w", m, err)
		}
		coreTables[table] = true
	}

	type owner struct {
		plugin string // Name() of the declaring plugin
		typ    string // fully qualified struct type
	}
	claimed := make(map[string]owner)
	for _, p := range plugins {
		for _, m := range p.Models() {
			table, typ, err := tableOf(m)
			if err != nil {
				return fmt.Errorf("preflight: parse model %T from plugin %q: %w", m, p.Name(), err)
			}
			if coreTables[table] {
				continue // mirroring a core table is the documented convention
			}
			if prev, ok := claimed[table]; ok && prev.typ != typ {
				return fmt.Errorf(
					"preflight: table %q is declared by two different model types: %s (plugin %q) and %s (plugin %q) — rename one model or its TableName() so each plugin-owned table has exactly one definition",
					table, prev.typ, prev.plugin, typ, p.Name(),
				)
			}
			claimed[table] = owner{plugin: p.Name(), typ: typ}
		}
	}
	return nil
}
