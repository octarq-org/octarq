package ontology_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/ontology"
)

func sqliteSpec() ontology.Spec {
	return ontology.Spec{
		Slug:  "widget",
		Label: "Widget",
		Fields: []ontology.Field{
			{Name: "name", Type: ontology.TypeString, Required: true},
			{Name: "count", Type: ontology.TypeInteger, Indexed: true},
			{Name: "code", Type: ontology.TypeString, Unique: true},
			{Name: "note", Type: ontology.TypeText},
		},
		Operations: ontology.AllowAllOperations(),
	}
}

func TestDialectFor(t *testing.T) {
	for _, name := range []string{"sqlite", "SQLite3", "postgres", "postgresql", "pgx", "mysql", " mysql "} {
		if _, err := ontology.DialectFor(name); err != nil {
			t.Errorf("DialectFor(%q) error: %v", name, err)
		}
	}
	if _, err := ontology.DialectFor("oracle"); err == nil {
		t.Fatal("DialectFor(oracle) = nil error, want unknown dialect")
	} else if !strings.Contains(err.Error(), "unknown dialect") {
		t.Fatalf("err = %v", err)
	}
}

func TestColumnTypePerDialect(t *testing.T) {
	want := map[string]map[ontology.FieldType]string{
		"sqlite": {
			ontology.TypeString: "TEXT", ontology.TypeText: "TEXT", ontology.TypeInteger: "INTEGER",
			ontology.TypeBoolean: "INTEGER", ontology.TypeNumber: "REAL", ontology.TypeTimestamp: "DATETIME",
			ontology.TypeUUID: "TEXT", ontology.TypeJSON: "TEXT",
		},
		"postgres": {
			ontology.TypeString: "VARCHAR(255)", ontology.TypeText: "TEXT", ontology.TypeInteger: "BIGINT",
			ontology.TypeBoolean: "BOOLEAN", ontology.TypeNumber: "DOUBLE PRECISION", ontology.TypeTimestamp: "TIMESTAMPTZ",
			ontology.TypeUUID: "UUID", ontology.TypeJSON: "JSONB",
		},
		"mysql": {
			ontology.TypeString: "VARCHAR(255)", ontology.TypeText: "TEXT", ontology.TypeInteger: "BIGINT",
			ontology.TypeBoolean: "TINYINT(1)", ontology.TypeNumber: "DOUBLE", ontology.TypeTimestamp: "DATETIME",
			ontology.TypeUUID: "CHAR(36)", ontology.TypeJSON: "JSON",
		},
	}
	for name, types := range want {
		d, err := ontology.DialectFor(name)
		if err != nil {
			t.Fatalf("DialectFor(%q): %v", name, err)
		}
		for ft, expected := range types {
			got, err := d.ColumnType(ft)
			if err != nil {
				t.Fatalf("%s.ColumnType(%s): %v", name, ft, err)
			}
			if got != expected {
				t.Errorf("%s.ColumnType(%s) = %q, want %q", name, ft, got, expected)
			}
		}
		if _, err := d.ColumnType("blob"); err == nil {
			t.Errorf("%s.ColumnType(blob) = nil error", name)
		}
	}
}

func TestCreateTableSQLSQLite(t *testing.T) {
	d, _ := ontology.DialectFor("sqlite")
	stmts, err := ontology.CreateTableSQL(d, sqliteSpec())
	if err != nil {
		t.Fatalf("CreateTableSQL: %v", err)
	}

	wantTable := `CREATE TABLE "ont_widget" (
  "id" TEXT PRIMARY KEY,
  "org_id" INTEGER NOT NULL,
  "created_at" DATETIME NOT NULL,
  "updated_at" DATETIME NOT NULL,
  "name" TEXT NOT NULL,
  "count" INTEGER,
  "code" TEXT,
  "note" TEXT
)`
	if stmts[0] != wantTable {
		t.Errorf("table DDL:\n%s\nwant:\n%s", stmts[0], wantTable)
	}

	want := []string{
		`CREATE INDEX IF NOT EXISTS "idx_ont_widget_org_id" ON "ont_widget" ("org_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ont_widget_org_id_count" ON "ont_widget" ("org_id", "count")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uq_ont_widget_org_id_code" ON "ont_widget" ("org_id", "code")`,
	}
	if len(stmts) != 1+len(want) {
		t.Fatalf("got %d statements, want %d: %v", len(stmts), 1+len(want), stmts)
	}
	for i, w := range want {
		if stmts[i+1] != w {
			t.Errorf("index %d = %q, want %q", i, stmts[i+1], w)
		}
	}
}

func TestCreateTableSQLPostgresAndMySQL(t *testing.T) {
	pg, _ := ontology.DialectFor("postgres")
	pgStmts, err := ontology.CreateTableSQL(pg, sqliteSpec())
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	if !strings.Contains(pgStmts[0], `"count" BIGINT`) {
		t.Errorf("postgres table DDL missing BIGINT: %s", pgStmts[0])
	}
	if !strings.Contains(pgStmts[0], `"org_id" BIGINT NOT NULL`) {
		t.Errorf("postgres table DDL missing tenant column: %s", pgStmts[0])
	}

	my, _ := ontology.DialectFor("mysql")
	myStmts, err := ontology.CreateTableSQL(my, sqliteSpec())
	if err != nil {
		t.Fatalf("mysql: %v", err)
	}
	if !strings.Contains(myStmts[0], "`count` BIGINT") {
		t.Errorf("mysql table DDL missing backticks: %s", myStmts[0])
	}
	for _, stmt := range myStmts[1:] {
		if strings.Contains(stmt, "IF NOT EXISTS") {
			t.Errorf("mysql does not support %q: %s", "IF NOT EXISTS", stmt)
		}
	}
}

func TestCreateTableSQLRefusesBadInput(t *testing.T) {
	d, _ := ontology.DialectFor("sqlite")
	if _, err := ontology.CreateTableSQL(nil, sqliteSpec()); err == nil {
		t.Error("nil dialect = nil error")
	}
	bad := sqliteSpec()
	bad.Slug = "Not A Slug"
	if _, err := ontology.CreateTableSQL(d, bad); err == nil {
		t.Error("invalid slug = nil error")
	}
	badType := sqliteSpec()
	badType.Fields = append(badType.Fields, ontology.Field{Name: "raw", Type: "bytes"})
	if _, err := ontology.CreateTableSQL(d, badType); err == nil {
		t.Error("unknown field type = nil error")
	}
}

func TestAddColumnSQL(t *testing.T) {
	d, _ := ontology.DialectFor("sqlite")
	spec := sqliteSpec()

	got, err := ontology.AddColumnSQL(d, spec, ontology.Field{Name: "sku", Type: ontology.TypeString, Required: true, Default: strptr("n/a")})
	if err != nil {
		t.Fatalf("AddColumnSQL: %v", err)
	}
	want := `ALTER TABLE "ont_widget" ADD COLUMN "sku" TEXT NOT NULL DEFAULT 'n/a'`
	if got != want {
		t.Errorf("AddColumnSQL = %q, want %q", got, want)
	}

	// Numbers and booleans must not be quoted: SQLite would store them as text
	// and Postgres would reject the literal.
	num, err := ontology.AddColumnSQL(d, spec, ontology.Field{Name: "qty", Type: ontology.TypeInteger, Default: strptr("0")})
	if err != nil {
		t.Fatalf("AddColumnSQL integer: %v", err)
	}
	if !strings.HasSuffix(num, `"qty" INTEGER DEFAULT 0`) {
		t.Errorf("integer default rendered as %q", num)
	}
	flag, err := ontology.AddColumnSQL(d, spec, ontology.Field{Name: "on", Type: ontology.TypeBoolean, Default: strptr("false")})
	if err != nil {
		t.Fatalf("AddColumnSQL boolean: %v", err)
	}
	if !strings.HasSuffix(flag, `"on" INTEGER DEFAULT FALSE`) {
		t.Errorf("boolean default rendered as %q", flag)
	}

	if _, err := ontology.AddColumnSQL(nil, spec, ontology.Field{Name: "sku", Type: ontology.TypeString}); err == nil {
		t.Error("nil dialect = nil error")
	}
	if _, err := ontology.AddColumnSQL(d, ontology.Spec{Slug: "bad slug"}, ontology.Field{Name: "sku", Type: ontology.TypeString}); err == nil {
		t.Error("invalid slug = nil error")
	}
	if _, err := ontology.AddColumnSQL(d, spec, ontology.Field{Name: "raw", Type: "bytes"}); err == nil {
		t.Error("unknown type = nil error")
	}
}

func TestCreateIndexSQL(t *testing.T) {
	d, _ := ontology.DialectFor("postgres")

	got, err := ontology.CreateIndexSQL(d, "ont_widget", []string{"org_id", "code"}, true, "")
	if err != nil {
		t.Fatalf("CreateIndexSQL: %v", err)
	}
	want := `CREATE UNIQUE INDEX IF NOT EXISTS "uq_ont_widget_org_id_code" ON "ont_widget" ("org_id", "code")`
	if got != want {
		t.Errorf("CreateIndexSQL = %q, want %q", got, want)
	}

	named, err := ontology.CreateIndexSQL(d, "ont_widget", []string{"org_id"}, false, "custom")
	if err != nil {
		t.Fatalf("CreateIndexSQL named: %v", err)
	}
	if !strings.Contains(named, `"custom"`) {
		t.Errorf("explicit name not used: %s", named)
	}

	if _, err := ontology.CreateIndexSQL(nil, "t", []string{"c"}, false, ""); err == nil {
		t.Error("nil dialect = nil error")
	}
	if _, err := ontology.CreateIndexSQL(d, "", []string{"c"}, false, ""); err == nil {
		t.Error("empty table = nil error")
	}
	if _, err := ontology.CreateIndexSQL(d, "t", nil, false, ""); err == nil {
		t.Error("no columns = nil error")
	}
}

func TestQuoteEscapes(t *testing.T) {
	sq, _ := ontology.DialectFor("sqlite")
	if got := sq.Quote(`a"b`); got != `"a""b"` {
		t.Errorf("sqlite quote = %q", got)
	}
	my, _ := ontology.DialectFor("mysql")
	if got := my.Quote("a`b"); got != "`a``b`" {
		t.Errorf("mysql quote = %q", got)
	}
}

// userScopedSpec is sqliteSpec partitioned by the member who owns the row.
func userScopedSpec() ontology.Spec {
	s := sqliteSpec()
	s.Scope = ontology.ScopeUser
	return s
}

// TestUserScopedTableCarriesTheOwnerColumn pins the DDL half of owner-level
// isolation: the platform adds the column, and every index starts with it, so a
// unique field is unique inside one member's rows rather than across the whole
// workspace.
func TestUserScopedTableCarriesTheOwnerColumn(t *testing.T) {
	d, err := ontology.DialectFor("sqlite")
	if err != nil {
		t.Fatalf("DialectFor: %v", err)
	}
	spec := userScopedSpec()

	stmts, err := ontology.CreateTableSQL(d, spec)
	if err != nil {
		t.Fatalf("CreateTableSQL: %v", err)
	}
	if !strings.Contains(stmts[0], `"user_id" INTEGER NOT NULL`) {
		t.Fatalf("create table does not carry the owner column:\n%s", stmts[0])
	}

	want := []string{
		`CREATE INDEX IF NOT EXISTS "idx_ont_widget_org_id_user_id" ON "ont_widget" ("org_id", "user_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ont_widget_org_id_user_id_count" ON "ont_widget" ("org_id", "user_id", "count")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uq_ont_widget_org_id_user_id_code" ON "ont_widget" ("org_id", "user_id", "code")`,
	}
	for _, w := range want {
		if !slices.Contains(stmts, w) {
			t.Fatalf("missing statement %q, have:\n%s", w, strings.Join(stmts, "\n"))
		}
	}

	// The same declaration without a scope keeps the workspace-wide shape.
	orgStmts, err := ontology.CreateTableSQL(d, sqliteSpec())
	if err != nil {
		t.Fatalf("CreateTableSQL: %v", err)
	}
	if strings.Contains(orgStmts[0], `"user_id"`) {
		t.Fatalf("a workspace-scoped table gained an owner column:\n%s", orgStmts[0])
	}
}

// TestIndexNamesFollowTheScope pins that creation and reconciliation agree on
// the name of a scoped index. A reconciler that spelled the org-only name would
// miss the index the table actually carries and rebuild it on every boot.
func TestIndexNamesFollowTheScope(t *testing.T) {
	user := userScopedSpec()
	if got, want := ontology.TenantIndexName(user), "idx_ont_widget_org_id_user_id"; got != want {
		t.Fatalf("TenantIndexName = %q, want %q", got, want)
	}
	if got, want := ontology.FieldIndexName(user, user.Fields[2]), "uq_ont_widget_org_id_user_id_code"; got != want {
		t.Fatalf("FieldIndexName = %q, want %q", got, want)
	}
	org := sqliteSpec()
	if got, want := ontology.TenantIndexName(org), "idx_ont_widget_org_id"; got != want {
		t.Fatalf("TenantIndexName = %q, want %q", got, want)
	}
}
