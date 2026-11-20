package ontology

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"gorm.io/gorm"
)

var ErrMissingDB = errors.New("ontology: nil database")

var validIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Inspector interface {
	TableExists(ctx context.Context, table string) (bool, error)
	Columns(ctx context.Context, table string) (map[string]bool, error)
	Indexes(ctx context.Context, table string) (map[string]bool, error)
}

func InspectorFor(driver string, db *gorm.DB) (Inspector, error) {
	if db == nil {
		return nil, ErrMissingDB
	}
	d, err := DialectFor(driver)
	if err != nil {
		return nil, err
	}
	switch d.Name() {
	case "sqlite":
		return sqliteInspector{db: db}, nil
	case "postgres":
		return postgresInspector{db: db}, nil
	case "mysql":
		return mysqlInspector{db: db}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDialect, driver)
	}
}

func checkTableName(table string) error {
	if !validIdent.MatchString(table) {
		return fmt.Errorf("ontology: invalid table name %q", table)
	}
	return nil
}

type nameRows []struct {
	Name string `gorm:"column:name"`
}

func (r nameRows) set() map[string]bool {
	out := make(map[string]bool, len(r))
	for _, row := range r {
		out[row.Name] = true
	}
	return out
}

type sqliteInspector struct{ db *gorm.DB }

func (i sqliteInspector) Columns(ctx context.Context, table string) (map[string]bool, error) {
	if err := checkTableName(table); err != nil {
		return nil, err
	}
	var rows nameRows
	if err := i.db.WithContext(ctx).
		Raw(`SELECT name FROM pragma_table_info(?)`, table).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("ontology: read columns of %s: %w", table, err)
	}
	return rows.set(), nil
}

func (i sqliteInspector) Indexes(ctx context.Context, table string) (map[string]bool, error) {
	if err := checkTableName(table); err != nil {
		return nil, err
	}
	var rows nameRows
	if err := i.db.WithContext(ctx).
		Raw(`SELECT name FROM pragma_index_list(?)`, table).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("ontology: read indexes of %s: %w", table, err)
	}
	return rows.set(), nil
}

func (i sqliteInspector) TableExists(ctx context.Context, table string) (bool, error) {
	cols, err := i.Columns(ctx, table)
	if err != nil {
		return false, err
	}
	return len(cols) > 0, nil
}

type postgresInspector struct{ db *gorm.DB }

func (i postgresInspector) Columns(ctx context.Context, table string) (map[string]bool, error) {
	if err := checkTableName(table); err != nil {
		return nil, err
	}
	var rows nameRows
	if err := i.db.WithContext(ctx).Raw(`
		SELECT column_name AS name
		FROM information_schema.columns
		WHERE table_name = ? AND table_schema = current_schema()`, table).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("ontology: read columns of %s: %w", table, err)
	}
	return rows.set(), nil
}

func (i postgresInspector) Indexes(ctx context.Context, table string) (map[string]bool, error) {
	if err := checkTableName(table); err != nil {
		return nil, err
	}
	var rows nameRows
	if err := i.db.WithContext(ctx).Raw(`
		SELECT indexname AS name
		FROM pg_indexes
		WHERE tablename = ? AND schemaname = current_schema()`, table).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("ontology: read indexes of %s: %w", table, err)
	}
	return rows.set(), nil
}

func (i postgresInspector) TableExists(ctx context.Context, table string) (bool, error) {
	cols, err := i.Columns(ctx, table)
	if err != nil {
		return false, err
	}
	return len(cols) > 0, nil
}

type mysqlInspector struct{ db *gorm.DB }

func (i mysqlInspector) Columns(ctx context.Context, table string) (map[string]bool, error) {
	if err := checkTableName(table); err != nil {
		return nil, err
	}
	var rows nameRows
	if err := i.db.WithContext(ctx).Raw(`
		SELECT column_name AS name
		FROM information_schema.columns
		WHERE table_name = ? AND table_schema = DATABASE()`, table).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("ontology: read columns of %s: %w", table, err)
	}
	return rows.set(), nil
}

func (i mysqlInspector) Indexes(ctx context.Context, table string) (map[string]bool, error) {
	if err := checkTableName(table); err != nil {
		return nil, err
	}
	var rows nameRows
	if err := i.db.WithContext(ctx).Raw(`
		SELECT DISTINCT index_name AS name
		FROM information_schema.statistics
		WHERE table_name = ? AND table_schema = DATABASE()`, table).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("ontology: read indexes of %s: %w", table, err)
	}
	return rows.set(), nil
}

func (i mysqlInspector) TableExists(ctx context.Context, table string) (bool, error) {
	cols, err := i.Columns(ctx, table)
	if err != nil {
		return false, err
	}
	return len(cols) > 0, nil
}
