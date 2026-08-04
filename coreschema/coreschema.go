// Package coreschema exposes the core database schema to out-of-tree plugins,
// so a plugin can verify in its own tests that the tables it reads still look
// the way it thinks they do.
//
// # Why this exists
//
// A plugin that needs a core table it doesn't own declares a small local struct
// with a matching TableName() — a "mirror" — rather than importing core's
// models. That keeps the coupling to a table name instead of a Go type, which
// is the point. What it doesn't do is fail loudly: rename a core column and the
// mirror still compiles, still migrates in the plugin's own tests, and only
// breaks at runtime as a SQL error on a path nobody exercised before release.
//
// The plugin's tests can't catch it either, because they AutoMigrate the mirror
// struct — the table gets created from the plugin's own copy, so the copy is
// correct by construction no matter how far core has moved.
//
// This package closes that: [Models] builds the real core schema, and
// [CheckMirror] compares a mirror against it column by column.
//
// # Why Models returns []any
//
// Deliberately opaque. Callers can hand these to AutoMigrate but cannot name
// the types without importing core's internal models package, which is not
// importable from outside this module. Migrating against core's schema is
// supported; reaching into core's structs is not — that is the coupling mirrors
// exist to avoid, and this package must not become a way around it.
package coreschema

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
)

// Models returns the core models as opaque values, ready for AutoMigrate. The
// set matches what the application migrates at startup, so a table created from
// these is the table a plugin will meet in production.
func Models() []any {
	return models.AllModels()
}

// CheckMirror reports whether every column mirror declares exists in the core
// table it names. db must already have the core schema migrated (see [Models]).
//
// The check is deliberately one-directional. A mirror is allowed to declare
// fewer columns than the real table — that is the normal case, since a mirror
// names only the handful of columns its queries touch. It is not allowed to
// declare a column the table doesn't have: that column is either misspelled or
// has been renamed out from under it, and every query using it will fail at
// runtime.
//
// Types are not compared. GORM maps the same Go type to different SQL types per
// driver, so a type assertion here would fail on Postgres for a schema that is
// fine on SQLite, and the failure would say nothing useful. Column existence is
// the drift that actually happens and the drift that silently breaks queries.
func CheckMirror(db *gorm.DB, mirror any) error {
	if db == nil {
		return fmt.Errorf("coreschema: nil db")
	}

	parsed, err := gormschema.Parse(mirror, &sync.Map{}, db.NamingStrategy)
	if err != nil {
		return fmt.Errorf("coreschema: parse mirror %T: %w", mirror, err)
	}
	table := parsed.Table
	if table == "" {
		return fmt.Errorf("coreschema: mirror %T resolves to an empty table name", mirror)
	}

	if !db.Migrator().HasTable(table) {
		return fmt.Errorf("coreschema: mirror %T names table %q, which does not exist in the core schema — "+
			"either the table was renamed or it is not a core table at all", mirror, table)
	}

	columnTypes, err := db.Migrator().ColumnTypes(table)
	if err != nil {
		return fmt.Errorf("coreschema: read columns of %q: %w", table, err)
	}
	actual := make(map[string]bool, len(columnTypes))
	for _, c := range columnTypes {
		actual[strings.ToLower(c.Name())] = true
	}

	var missing []string
	for _, f := range parsed.Fields {
		// Fields with no DB name are ignored by GORM too (gorm:"-", or a plain
		// embedded struct), so they cannot drift.
		if f.DBName == "" {
			continue
		}
		if !actual[strings.ToLower(f.DBName)] {
			missing = append(missing, f.DBName)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)

	have := make([]string, 0, len(actual))
	for name := range actual {
		have = append(have, name)
	}
	sort.Strings(have)

	return fmt.Errorf("coreschema: mirror %T is out of date with core table %q: "+
		"declares column(s) %s, which the table does not have (table has: %s)",
		mirror, table, strings.Join(missing, ", "), strings.Join(have, ", "))
}
