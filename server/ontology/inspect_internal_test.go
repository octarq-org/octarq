package ontology

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestEveryInspectorReportsAFailedQueryAsAnError pins the error path of each
// dialect. The postgres and mysql inspectors cannot reach a live server in this
// suite, but they must not answer "no columns" when the query itself failed: an
// empty table and an unreachable database would look the same, and the
// difference decides whether a declaration is applied or skipped.
func TestEveryInspectorReportsAFailedQueryAsAnError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	for _, driver := range []string{"sqlite", "postgres", "mysql"} {
		t.Run(driver, func(t *testing.T) {
			insp, err := InspectorFor(driver, db)
			if err != nil {
				t.Fatalf("InspectorFor(%q): %v", driver, err)
			}
			ctx := context.Background()

			if _, err := insp.TableExists(ctx, "ont_tickets"); err == nil {
				t.Fatal("TableExists on a closed db = nil error, want a failure")
			}
			if _, err := insp.Columns(ctx, "ont_tickets"); err == nil {
				t.Fatal("Columns on a closed db = nil error, want a failure")
			}
			if _, err := insp.Indexes(ctx, "ont_tickets"); err == nil {
				t.Fatal("Indexes on a closed db = nil error, want a failure")
			}

			// A table name is interpolated into each dialect's query, so an
			// invalid one is refused before it is sent.
			if _, err := insp.Columns(ctx, "ont tickets; drop"); err == nil {
				t.Fatal("Columns with an invalid name = nil error, want a refusal")
			}
		})
	}

	if _, err := InspectorFor("sqlite", nil); err == nil {
		t.Fatal("InspectorFor with no db = nil error, want ErrMissingDB")
	}
	if _, err := InspectorFor("oracle", db); err == nil {
		t.Fatal("InspectorFor with an unknown driver = nil error, want a refusal")
	}
}
