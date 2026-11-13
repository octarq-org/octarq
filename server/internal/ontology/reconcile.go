package ontology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrUnmanagedTable is returned when an entity table exists but no bookkeeping
// row says which declaration created it. Without the row there is nothing to
// diff against, and guessing from column metadata is unsafe (SQLite stores
// string, text, uuid and json all as TEXT).
var ErrUnmanagedTable = errors.New("ontology: entity table exists without a bookkeeping row")

// ErrBookkeepingMissing is returned when the bookkeeping table itself is absent.
// The MCP composition does not run migrations, so this is the expected state
// there: a server start is what applies schema.
var ErrBookkeepingMissing = errors.New("ontology: schema was never applied by a server run")

// Step is one entity's reconciliation result: the statements to run, plus the
// bookkeeping row to write once they succeed.
type Step struct {
	Slug       string
	Table      string
	Statements []string
	// Dirty reports whether the declaration differs from the stored one. Only a
	// dirty entity rewrites its bookkeeping row, so AppliedAt keeps meaning
	// "when this declaration took effect" rather than "when the server last
	// booted".
	Dirty   bool
	existed bool // a bookkeeping row was already present
	row     AppliedSpec
}

// Plan computes what must happen to every declared entity without writing
// anything. Boot plans first and executes second, so a declaration that cannot
// be applied leaves the database untouched instead of half-migrated.
//
// driver is the OCTARQ_DB_DRIVER value; db is used for reads only.
func Plan(ctx context.Context, db *gorm.DB, driver string, specs []Spec) ([]Step, error) {
	dial, err := DialectFor(driver)
	if err != nil {
		return nil, err
	}
	insp, err := InspectorFor(driver, db)
	if err != nil {
		return nil, err
	}

	exists, err := insp.TableExists(ctx, AppliedSpecTable)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrBookkeepingMissing
	}

	applied := make(map[string]AppliedSpec, len(specs))
	slugs := SortedSlugs(specs)
	if len(slugs) > 0 {
		var rows []AppliedSpec
		if err := db.WithContext(ctx).Where("slug IN ?", slugs).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("ontology: read applied specs: %w", err)
		}
		for _, row := range rows {
			applied[row.Slug] = row
		}
	}

	steps := make([]Step, 0, len(specs))
	for _, spec := range sortedSpecs(specs) {
		step, err := planOne(ctx, db, dial, insp, spec, applied[spec.Slug])
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func sortedSpecs(specs []Spec) []Spec {
	out := append([]Spec(nil), specs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func planOne(ctx context.Context, db *gorm.DB, dial Dialect, insp Inspector, spec Spec, applied AppliedSpec) (Step, error) {
	table := TableName(spec.Slug)

	hash, err := spec.Hash()
	if err != nil {
		return Step{}, fmt.Errorf("ontology: hash %s: %w", spec.Slug, err)
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return Step{}, fmt.Errorf("ontology: encode %s: %w", spec.Slug, err)
	}
	step := Step{
		Slug:  spec.Slug,
		Table: table,
		row: AppliedSpec{
			Slug:      spec.Slug,
			SpecJSON:  string(raw),
			Hash:      hash,
			AppliedAt: time.Now().UTC(),
		},
	}

	exists, err := insp.TableExists(ctx, table)
	if err != nil {
		return Step{}, err
	}
	if !exists {
		stmts, err := CreateTableSQL(dial, spec)
		if err != nil {
			return Step{}, err
		}
		step.Statements = stmts
		return step, nil
	}

	if applied.Slug == "" {
		return Step{}, fmt.Errorf("%w: %s (%s)", ErrUnmanagedTable, table, spec.Slug)
	}
	step.existed = true

	var old Spec
	if err := json.Unmarshal([]byte(applied.SpecJSON), &old); err != nil {
		return Step{}, fmt.Errorf("ontology: decode applied spec for %s: %w", spec.Slug, err)
	}
	step.Dirty = applied.Hash != hash

	// The row count only matters when the declaration changed: it is what turns
	// "drop a column" from a rewrite into data loss. An unchanged declaration
	// still gets its indexes checked, because a manual DROP INDEX must be
	// repaired on the next boot.
	if step.Dirty {
		rows, err := countRows(ctx, db, table)
		if err != nil {
			return Step{}, err
		}
		if err := ValidateTransition(old, spec, rows); err != nil {
			return Step{}, fmt.Errorf("ontology: %s: %w", spec.Slug, err)
		}
	}

	cols, err := insp.Columns(ctx, table)
	if err != nil {
		return Step{}, err
	}
	idx, err := insp.Indexes(ctx, table)
	if err != nil {
		return Step{}, err
	}

	known := make(map[string]bool, len(old.Fields))
	for _, f := range old.Fields {
		known[f.Name] = true
	}
	for _, f := range spec.Fields {
		if known[f.Name] {
			continue
		}
		// The bookkeeping and the physical table can disagree after a manual
		// intervention. Treat a column that already exists as present rather
		// than emitting an ALTER the database will reject.
		if cols[f.Name] {
			continue
		}
		stmt, err := AddColumnSQL(dial, spec, f)
		if err != nil {
			return Step{}, err
		}
		step.Statements = append(step.Statements, stmt)
	}

	// An index the bookkeeping says exists but the database lost (a manual drop,
	// a restore from a partial dump) is re-created. The reverse case is refused
	// by ValidateTransition.
	if !idx[TenantIndexName(table)] {
		stmt, err := CreateIndexSQL(dial, table, []string{ColumnOrgID}, false, TenantIndexName(table))
		if err != nil {
			return Step{}, err
		}
		step.Statements = append(step.Statements, stmt)
	}
	// Driven by the new declaration, not the stored one: a field added in this
	// run gets its index in the same run, and a field that just gained
	// unique/indexed gets one too. Adding an index the old declaration did not
	// carry is always safe; removing one is refused above.
	for _, f := range spec.Fields {
		if !f.Unique && !f.Indexed {
			continue
		}
		name := FieldIndexName(table, f)
		if idx[name] {
			continue
		}
		stmt, err := CreateIndexSQL(dial, table, []string{ColumnOrgID, f.Name}, f.Unique, name)
		if err != nil {
			return Step{}, err
		}
		step.Statements = append(step.Statements, stmt)
	}

	return step, nil
}

func countRows(ctx context.Context, db *gorm.DB, table string) (int64, error) {
	if err := checkTableName(table); err != nil {
		return 0, err
	}
	var n int64
	if err := db.WithContext(ctx).Table(table).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("ontology: count %s: %w", table, err)
	}
	return n, nil
}

// Apply plans and then executes. A plan error writes nothing.
func Apply(ctx context.Context, db *gorm.DB, driver string, specs []Spec) error {
	if len(specs) == 0 {
		return nil
	}
	steps, err := Plan(ctx, db, driver, specs)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if len(step.Statements) == 0 {
			continue
		}
		for _, stmt := range step.Statements {
			if err := db.WithContext(ctx).Exec(stmt).Error; err != nil {
				return fmt.Errorf("ontology: apply %s: %w (statement: %s)", step.Slug, err, stmt)
			}
		}
	}
	return record(ctx, db, steps)
}

func record(ctx context.Context, db *gorm.DB, steps []Step) error {
	for _, step := range steps {
		if step.existed && !step.Dirty {
			continue
		}
		row := step.row
		if err := db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "slug"}},
				UpdateAll: true,
			}).
			Create(&row).Error; err != nil {
			return fmt.Errorf("ontology: record applied spec for %s: %w", step.Slug, err)
		}
		slog.Debug("ontology: entity applied", "slug", step.Slug, "statements", len(step.Statements))
	}
	return nil
}
