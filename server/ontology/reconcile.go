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

var ErrUnmanagedTable = errors.New("ontology: entity table exists without a bookkeeping row")

var ErrBookkeepingMissing = errors.New("ontology: schema was never applied by a server run")

type Step struct {
	Slug       string
	Table      string
	Statements []string
	Dirty      bool
	existed    bool
	row        AppliedSpec
}

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

	for _, f := range spec.Fields {
		if cols[f.Name] {
			continue
		}
		stmt, err := AddColumnSQL(dial, spec, f)
		if err != nil {
			return Step{}, err
		}
		step.Statements = append(step.Statements, stmt)
	}

	if !idx[TenantIndexName(spec)] {
		stmt, err := CreateIndexSQL(dial, table, ScopeColumns(spec), false, TenantIndexName(spec))
		if err != nil {
			return Step{}, err
		}
		step.Statements = append(step.Statements, stmt)
	}
	for _, f := range spec.Fields {
		if !f.Unique && !f.Indexed {
			continue
		}
		name := FieldIndexName(spec, f)
		if idx[name] {
			continue
		}
		stmt, err := CreateIndexSQL(dial, table, append(ScopeColumns(spec), f.Name), f.Unique, name)
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

func Apply(ctx context.Context, db *gorm.DB, driver string, specs []Spec) error {
	if len(specs) == 0 {
		return nil
	}
	steps, err := Plan(ctx, db, driver, specs)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := applyStep(ctx, db, step); err != nil {
			return err
		}
	}
	return nil
}

func applyStep(ctx context.Context, db *gorm.DB, step Step) error {
	for _, stmt := range step.Statements {
		if err := db.WithContext(ctx).Exec(stmt).Error; err != nil {
			if !step.existed && db.WithContext(ctx).Migrator().HasTable(step.Table) {
				if recErr := record(ctx, db, step); recErr != nil {
					return fmt.Errorf("ontology: apply %s: %w (statement: %s); record: %w",
						step.Slug, err, stmt, recErr)
				}
			}
			return fmt.Errorf("ontology: apply %s: %w (statement: %s)", step.Slug, err, stmt)
		}
	}
	if step.existed && !step.Dirty {
		return nil
	}
	return record(ctx, db, step)
}

func record(ctx context.Context, db *gorm.DB, step Step) error {
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
	return nil
}
