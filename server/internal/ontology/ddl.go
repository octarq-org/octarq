package ontology

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Dialect renders the three SQL statements the ontology layer needs. Postgres
// and SQLite share almost all of their syntax; MySQL differs in identifier
// quoting and in what a column type may promise.
type Dialect interface {
	// Name is the dialect's identifier: "sqlite", "postgres" or "mysql".
	Name() string
	// Quote wraps an identifier in the dialect's quote characters.
	Quote(ident string) string
	// ColumnType maps a declared field type to a column definition.
	ColumnType(FieldType) (string, error)
	// SupportsIfNotExists reports whether the dialect accepts
	// CREATE INDEX IF NOT EXISTS. MySQL does not.
	SupportsIfNotExists() bool
}

// ErrUnknownDialect is returned when no builder matches the driver name.
var ErrUnknownDialect = errors.New("ontology: unknown dialect")

// DialectFor returns the builder for a driver name as it appears in
// OCTARQ_DB_DRIVER.
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

// PlatformColumns are the columns every generated table carries. A declaration
// cannot name them (see ReservedColumnNames).
const (
	ColumnID        = "id"
	ColumnOrgID     = "org_id"
	ColumnCreatedAt = "created_at"
	ColumnUpdatedAt = "updated_at"
)

// maxIdentLen is the smallest identifier limit across the three dialects
// (Postgres and MySQL both stop at 63/64 bytes). Longer names are truncated and
// suffixed with a hash so two long names never collapse into one.
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

// TenantIndexName is the name of the (org_id) index every entity table carries.
// Creation and reconciliation call this same function on purpose: a name longer
// than maxIdentLen gains a hash suffix, so a reconciler that spelled
// "idx_<table>_<column>" by hand would miss the index actually created and
// re-create it on every boot.
func TenantIndexName(table string) string { return indexName("idx", table, ColumnOrgID) }

// FieldIndexName is the name of the index for one declared field. The index is
// always (org_id, field), so a tenant's rows never collide across tenants.
func FieldIndexName(table string, f Field) string {
	prefix := "idx"
	if f.Unique {
		prefix = "uq"
	}
	return indexName(prefix, table, ColumnOrgID+"_"+f.Name)
}

// CreateTableSQL returns the statements that create an entity's table: the table
// itself, the tenant index, and one index per declared index or unique field.
// The caller runs them in order.
func CreateTableSQL(d Dialect, spec Spec) ([]string, error) {
	if d == nil {
		return nil, errors.New("ontology: nil dialect")
	}
	if !ValidSlug(spec.Slug) {
		return nil, fmt.Errorf("ontology: refusing to build DDL for slug %q", spec.Slug)
	}
	table := TableName(spec.Slug)

	cols := []string{
		d.Quote(ColumnID) + " " + quoteColumnType(d, TypeString) + " PRIMARY KEY",
		d.Quote(ColumnOrgID) + " " + quoteColumnType(d, TypeInteger) + " NOT NULL",
		d.Quote(ColumnCreatedAt) + " " + quoteColumnType(d, TypeTimestamp) + " NOT NULL",
		d.Quote(ColumnUpdatedAt) + " " + quoteColumnType(d, TypeTimestamp) + " NOT NULL",
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

	tenantIdx, err := CreateIndexSQL(d, table, []string{ColumnOrgID}, false, "")
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, tenantIdx)

	for _, f := range spec.Fields {
		if !f.Indexed && !f.Unique {
			continue
		}
		stmt, err := CreateIndexSQL(d, table, []string{ColumnOrgID, f.Name}, f.Unique, "")
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

func quoteColumnType(d Dialect, t FieldType) string {
	ct, err := d.ColumnType(t)
	if err != nil {
		return "TEXT"
	}
	return ct
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

// defaultLiteral renders a declared default for its column type. Numbers stay
// bare and booleans become the SQL keywords, so SQLite's INTEGER, Postgres's
// BOOLEAN and MySQL's TINYINT(1) all receive a value of the right kind rather
// than a string that the driver would coerce.
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

// AddColumnSQL returns the statement that adds one field to an existing table.
// When the table already holds rows the column carries its declared default, so
// the ALTER cannot fail on a NOT NULL constraint.
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

// CreateIndexSQL returns a CREATE INDEX statement. Pass columns in query order:
// for a tenant-scoped index that means org_id first. name may be empty, in which
// case it is derived from the table and columns.
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
