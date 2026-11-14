package ontology_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/ontology"
	"gorm.io/gorm"
)

// TestGeneratedDDLRunsOnSQLite proves the builder's output is real SQL, not just
// a string that matches a golden file.
func TestGeneratedDDLRunsOnSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	d, err := ontology.DialectFor("sqlite")
	if err != nil {
		t.Fatalf("DialectFor: %v", err)
	}

	spec := sqliteSpec()
	stmts, err := ontology.CreateTableSQL(d, spec)
	if err != nil {
		t.Fatalf("CreateTableSQL: %v", err)
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	// Running the same statements again must be a no-op: CREATE TABLE fails, but
	// the indexes carry IF NOT EXISTS, which is what makes a re-apply safe.
	for _, stmt := range stmts[1:] {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("re-exec %q: %v", stmt, err)
		}
	}

	add, err := ontology.AddColumnSQL(d, spec, ontology.Field{Name: "sku", Type: ontology.TypeString})
	if err != nil {
		t.Fatalf("AddColumnSQL: %v", err)
	}
	if err := db.Exec(add).Error; err != nil {
		t.Fatalf("exec add column: %v", err)
	}

	if err := db.Exec(`INSERT INTO ont_widget (id, org_id, created_at, updated_at, name, code, sku) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"w1", 7, "2026-01-01", "2026-01-01", "gizmo", "G-1", "S-1").Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	var name string
	if err := db.Table("ont_widget").Select("name").Where("org_id = ?", 7).Row().Scan(&name); err != nil {
		t.Fatalf("select: %v", err)
	}
	if name != "gizmo" {
		t.Fatalf("name = %q, want gizmo", name)
	}
}
