package ontology

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type Dialect interface {
	Name() string
	Quote(ident string) string
	ColumnType(FieldType) (string, error)
	SupportsIfNotExists() bool
}

var ErrUnknownDialect = errors.New("ontology: unknown dialect")

func DialectFor(driver string) (Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite", "sqlite3":
		return sqliteDialect{}, nil
	case "postgres", "postgresql", "pgx":
		return postgresDialect{}, nil
	case "mysql":
		return mysqlDialect{}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDialect, driver)
	}
}

type sqliteDialect struct{}
type postgresDialect struct{}
type mysqlDialect struct{}

func (sqliteDialect) Name() string              { return "sqlite" }
func (sqliteDialect) Quote(id string) string    { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }
func (sqliteDialect) SupportsIfNotExists() bool { return true }

func (postgresDialect) Name() string              { return "postgres" }
func (postgresDialect) Quote(id string) string    { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }
func (postgresDialect) SupportsIfNotExists() bool { return true }

func (mysqlDialect) Name() string              { return "mysql" }
func (mysqlDialect) Quote(id string) string    { return "`" + strings.ReplaceAll(id, "`", "``") + "`" }
func (mysqlDialect) SupportsIfNotExists() bool { return false }

var columnTypes = map[string]map[FieldType]string{
	"sqlite": {
		TypeString:    "TEXT",
		TypeText:      "TEXT",
		TypeInteger:   "INTEGER",
		TypeBoolean:   "INTEGER",
		TypeNumber:    "REAL",
		TypeTimestamp: "DATETIME",
		TypeUUID:      "TEXT",
		TypeJSON:      "TEXT",
	},
	"postgres": {
		TypeString:    "VARCHAR(255)",
		TypeText:      "TEXT",
		TypeInteger:   "BIGINT",
		TypeBoolean:   "BOOLEAN",
		TypeNumber:    "DOUBLE PRECISION",
		TypeTimestamp: "TIMESTAMPTZ",
		TypeUUID:      "UUID",
		TypeJSON:      "JSONB",
	},
	"mysql": {
		TypeString:    "VARCHAR(255)",
		TypeText:      "TEXT",
		TypeInteger:   "BIGINT",
		TypeBoolean:   "TINYINT(1)",
		TypeNumber:    "DOUBLE",
		TypeTimestamp: "DATETIME",
		TypeUUID:      "CHAR(36)",
		TypeJSON:      "JSON",
	},
}

func columnType(dialect string, t FieldType) (string, error) {
	types, ok := columnTypes[dialect]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownDialect, dialect)
	}
	ct, ok := types[t]
	if !ok {
		return "", fmt.Errorf("ontology: %q has no column type in the %s dialect", t, dialect)
	}
	return ct, nil
}

func (sqliteDialect) ColumnType(t FieldType) (string, error)   { return columnType("sqlite", t) }
func (postgresDialect) ColumnType(t FieldType) (string, error) { return columnType("postgres", t) }
func (mysqlDialect) ColumnType(t FieldType) (string, error)    { return columnType("mysql", t) }

const (
	ColumnID        = "id"
	ColumnOrgID     = "org_id"
	ColumnUserID    = "user_id"
	ColumnCreatedAt = "created_at"
	ColumnUpdatedAt = "updated_at"
)

func ScopeColumns(spec Spec) []string {
	cols := []string{ColumnOrgID}
	if ScopeOf(spec) == ScopeUser {
		cols = append(cols, ColumnUserID)
	}
	return cols
}

const maxIdentLen = 63

func indexName(prefix, table, column string) string {
	name := prefix + "_" + table + "_" + column
	if len(name) <= maxIdentLen {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(sum[:4])
	keep := maxIdentLen - len(suffix) - 1
	return name[:keep] + "_" + suffix
}

func TenantIndexName(spec Spec) string {
	return indexName("idx", TableName(spec.Slug), strings.Join(ScopeColumns(spec), "_"))
}

func FieldIndexName(spec Spec, f Field) string {
	prefix := "idx"
	if f.Unique {
		prefix = "uq"
	}
	cols := append(ScopeColumns(spec), f.Name)
	return indexName(prefix, TableName(spec.Slug), strings.Join(cols, "_"))
}

type platformColumn struct {
	name       string
	typ        FieldType
	constraint string
}

func CreateTableSQL(d Dialect, spec Spec) ([]string, error) {
	if d == nil {
		return nil, errors.New("ontology: nil dialect")
	}
	if !ValidSlug(spec.Slug) {
		return nil, fmt.Errorf("ontology: refusing to build DDL for slug %q", spec.Slug)
	}
	table := TableName(spec.Slug)

	platform := []platformColumn{
		{ColumnID, TypeString, "PRIMARY KEY"},
		{ColumnOrgID, TypeInteger, "NOT NULL"},
	}
	if ScopeOf(spec) == ScopeUser {
		platform = append(platform, platformColumn{ColumnUserID, TypeInteger, "NOT NULL"})
	}
	platform = append(platform,
		platformColumn{ColumnCreatedAt, TypeTimestamp, "NOT NULL"},
		platformColumn{ColumnUpdatedAt, TypeTimestamp, "NOT NULL"},
	)
	cols := make([]string, 0, len(platform)+len(spec.Fields))
	for _, p := range platform {
		ct, err := d.ColumnType(p.typ)
		if err != nil {
			return nil, err
		}
		cols = append(cols, d.Quote(p.name)+" "+ct+" "+p.constraint)
	}
	for _, f := range spec.Fields {
		def, err := columnDefinition(d, f)
		if err != nil {
			return nil, err
		}
		cols = append(cols, d.Quote(f.Name)+" "+def)
	}

	stmts := []string{
		"CREATE TABLE " + d.Quote(table) + " (\n  " + strings.Join(cols, ",\n  ") + "\n)",
	}

	tenantIdx, err := CreateIndexSQL(d, table, ScopeColumns(spec), false, "")
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, tenantIdx)

	for _, f := range spec.Fields {
		if !f.Indexed && !f.Unique {
			continue
		}
		stmt, err := CreateIndexSQL(d, table, append(ScopeColumns(spec), f.Name), f.Unique, "")
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

func columnDefinition(d Dialect, f Field) (string, error) {
	ct, err := d.ColumnType(f.Type)
	if err != nil {
		return "", err
	}
	def := ct
	if f.Required {
		def += " NOT NULL"
	}
	if raw, ok := f.sqlDefault(); ok {
		def += " DEFAULT " + defaultLiteral(f.Type, raw)
	}
	return def, nil
}

func defaultLiteral(t FieldType, raw string) string {
	raw = strings.TrimSpace(raw)
	switch t {
	case TypeInteger, TypeNumber:
		return raw
	case TypeBoolean:
		if raw == "true" {
			return "TRUE"
		}
		return "FALSE"
	default:
		return quoteLiteral(raw)
	}
}

func AddColumnSQL(d Dialect, spec Spec, f Field) (string, error) {
	if d == nil {
		return "", errors.New("ontology: nil dialect")
	}
	if !ValidSlug(spec.Slug) {
		return "", fmt.Errorf("ontology: refusing to build DDL for slug %q", spec.Slug)
	}
	def, err := columnDefinition(d, f)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + d.Quote(TableName(spec.Slug)) +
		" ADD COLUMN " + d.Quote(f.Name) + " " + def, nil
}

func CreateIndexSQL(d Dialect, table string, columns []string, unique bool, name string) (string, error) {
	if d == nil {
		return "", errors.New("ontology: nil dialect")
	}
	if table == "" || len(columns) == 0 {
		return "", errors.New("ontology: index needs a table and at least one column")
	}
	if name == "" {
		prefix := "idx"
		if unique {
			prefix = "uq"
		}
		name = indexName(prefix, table, strings.Join(columns, "_"))
	}

	quoted := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = d.Quote(c)
	}

	var b strings.Builder
	b.WriteString("CREATE ")
	if unique {
		b.WriteString("UNIQUE ")
	}
	b.WriteString("INDEX ")
	if d.SupportsIfNotExists() {
		b.WriteString("IF NOT EXISTS ")
	}
	b.WriteString(d.Quote(name))
	b.WriteString(" ON ")
	b.WriteString(d.Quote(table))
	b.WriteString(" (")
	b.WriteString(strings.Join(quoted, ", "))
	b.WriteString(")")
	return b.String(), nil
}
