package ontology_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/ontology"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ontology.AppliedSpec{}); err != nil {
		t.Fatalf("migrate bookkeeping: %v", err)
	}
	return db
}

func ticketSpec(fields ...ontology.Field) ontology.Spec {
	return ontology.Spec{
		Slug:       "tickets",
		Label:      "Ticket",
		Fields:     fields,
		Operations: ontology.AllowAllOperations(),
	}
}

func baseFields() []ontology.Field {
	return []ontology.Field{
		{Name: "title", Type: ontology.TypeString, Required: true, Unique: true, Indexed: true, ListColumn: true},
		{Name: "status", Type: ontology.TypeString},
	}
}

func TestApplyCreatesTableAndIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	spec := ticketSpec(baseFields()...)

	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{spec}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	insp, err := ontology.InspectorFor("sqlite", db)
	if err != nil {
		t.Fatalf("InspectorFor: %v", err)
	}
	exists, err := insp.TableExists(ctx, "ont_tickets")
	if err != nil || !exists {
		t.Fatalf("table missing: exists=%v err=%v", exists, err)
	}
	idx, err := insp.Indexes(ctx, "ont_tickets")
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	for _, want := range []string{
		ontology.TenantIndexName(spec),
		ontology.FieldIndexName(spec, spec.Fields[0]),
	} {
		if !idx[want] {
			t.Fatalf("index %q missing, have %v", want, idx)
		}
	}

	// The second plan must be empty: an unchanged declaration never re-runs DDL.
	steps, err := ontology.Plan(ctx, db, "sqlite", []ontology.Spec{spec})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 1 || len(steps[0].Statements) != 0 {
		t.Fatalf("second plan = %+v, want no statements", steps)
	}
}

func TestApplyAddsColumnAndKeepsRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	v1 := ticketSpec(baseFields()...)
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{v1}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}
	if err := db.Exec(`INSERT INTO ont_tickets (id, org_id, created_at, updated_at, title, status) VALUES (?,?,?,?,?,?)`,
		"t1", 1, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "first", "open").Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	v2 := ticketSpec(append(baseFields(), ontology.Field{
		Name: "priority", Type: ontology.TypeString, Default: ptr("normal"),
	})...)
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{v2}); err != nil {
		t.Fatalf("Apply v2: %v", err)
	}

	var priority string
	if err := db.Table("ont_tickets").Select("priority").Where("id = ?", "t1").Row().Scan(&priority); err != nil {
		t.Fatalf("read new column: %v", err)
	}
	if priority != "normal" {
		t.Fatalf("priority = %q, want the declared default", priority)
	}
}

func TestApplyRefusesTypeChange(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(baseFields()...)}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	fields := baseFields()
	fields[1].Type = ontology.TypeInteger
	err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(fields...)})
	assertIssues(t, err, ontology.CodeTypeChangeUnsupported)
}

func TestApplyRefusesRequiredChange(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(baseFields()...)}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	fields := baseFields()
	fields[1].Required = true
	err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(fields...)})
	assertIssues(t, err, ontology.CodeRequiredChangeUnsupported)
}

// TestApplyRefusesRelaxingRequired is the other direction of the same rule: the
// column keeps NOT NULL, so a write the new declaration permits would still be
// rejected by the database while the bookkeeping said it was allowed.
func TestApplyRefusesRelaxingRequired(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(baseFields()...)}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	fields := baseFields()
	fields[0].Required = false
	err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(fields...)})
	assertIssues(t, err, ontology.CodeRequiredChangeUnsupported)
}

// TestApplyRefusesDefaultChange pins that a changed default is refused rather
// than recorded. The column keeps its old DEFAULT, so a row inserted without the
// field would get the old value while the declaration promised the new one.
func TestApplyRefusesDefaultChange(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	v1 := ticketSpec(append(baseFields(), ontology.Field{
		Name: "priority", Type: ontology.TypeString, Default: ptr("normal"),
	})...)
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{v1}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	v2 := ticketSpec(append(baseFields(), ontology.Field{
		Name: "priority", Type: ontology.TypeString, Default: ptr("high"),
	})...)
	assertIssues(t, ontology.Apply(ctx, db, "sqlite", []ontology.Spec{v2}),
		ontology.CodeDefaultChangeUnsupported)
}

func TestApplyRefusesDroppingIndex(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	fields := baseFields()
	fields[1].Indexed = true
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(fields...)}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	relaxed := baseFields()
	err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(relaxed...)})
	assertIssues(t, err, ontology.CodeIndexRemoved)
}

func TestApplyRefusesDroppingUniqueIndex(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(baseFields()...)}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	fields := baseFields()
	fields[0].Unique = false
	err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(fields...)})
	assertIssues(t, err, ontology.CodeIndexRemoved)
}

func TestApplyRefusesUnmanagedTable(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.Exec(`CREATE TABLE ont_ghost (id TEXT PRIMARY KEY, org_id INTEGER NOT NULL,
		created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, name TEXT)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	spec := ticketSpec(baseFields()...)
	spec.Slug = "ghost"

	err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{spec})
	if !errors.Is(err, ontology.ErrUnmanagedTable) {
		t.Fatalf("err = %v, want ErrUnmanagedTable", err)
	}
}

func TestApplyRefusesWithoutBookkeepingTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	err = ontology.Apply(context.Background(), db, "sqlite", []ontology.Spec{ticketSpec(baseFields()...)})
	if !errors.Is(err, ontology.ErrBookkeepingMissing) {
		t.Fatalf("err = %v, want ErrBookkeepingMissing", err)
	}
}

func TestApplyRecreatesMissingIndex(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	spec := ticketSpec(baseFields()...)
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{spec}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	name := ontology.TenantIndexName(spec)
	if err := db.Exec(`DROP INDEX "` + name + `"`).Error; err != nil {
		t.Fatalf("drop index: %v", err)
	}

	steps, err := ontology.Plan(ctx, db, "sqlite", []ontology.Spec{spec})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps[0].Statements) != 1 {
		t.Fatalf("plan = %+v, want one CREATE INDEX", steps[0].Statements)
	}
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{spec}); err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	insp, err := ontology.InspectorFor("sqlite", db)
	if err != nil {
		t.Fatalf("InspectorFor: %v", err)
	}
	idx, err := insp.Indexes(ctx, "ont_tickets")
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	if !idx[name] {
		t.Fatalf("index %q was not recreated: %v", name, idx)
	}
}

func TestApplyRefusesDroppingField(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(baseFields()...)}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	// Not even an empty table allows it: DROP COLUMN would leave the index it
	// carried behind, and the bookkeeping row could not describe the result.
	err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(baseFields()[0])})
	assertIssues(t, err, ontology.CodeColumnDropUnsupported)
}

func TestApplyIndexesAddedField(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(baseFields()...)}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}
	v2 := ticketSpec(append(baseFields(), ontology.Field{
		Name: "code", Type: ontology.TypeString, Unique: true,
	})...)
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{v2}); err != nil {
		t.Fatalf("Apply v2: %v", err)
	}

	insp, err := ontology.InspectorFor("sqlite", db)
	if err != nil {
		t.Fatalf("InspectorFor: %v", err)
	}
	idx, err := insp.Indexes(ctx, "ont_tickets")
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	if want := ontology.FieldIndexName(v2, v2.Fields[2]); !idx[want] {
		t.Fatalf("index %q missing after adding a unique field: %v", want, idx)
	}
}

// TestApplyRepairsColumnDroppedByHand pins the drift repair. The declaration is
// the truth, so a column someone dropped manually is re-added on the next boot:
// otherwise every query against the entity fails at runtime, on a table the
// platform reports as applied.
func TestApplyRepairsColumnDroppedByHand(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	spec := ticketSpec(baseFields()...)
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{spec}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}
	if err := db.Exec(`ALTER TABLE ont_tickets DROP COLUMN status`).Error; err != nil {
		t.Fatalf("drop column by hand: %v", err)
	}

	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{spec}); err != nil {
		t.Fatalf("re-Apply: %v", err)
	}

	insp, err := ontology.InspectorFor("sqlite", db)
	if err != nil {
		t.Fatalf("InspectorFor: %v", err)
	}
	cols, err := insp.Columns(ctx, "ont_tickets")
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	if !cols["status"] {
		t.Fatalf("status was not re-added: %v", cols)
	}
}

func TestApplyDoesNotRewriteUnchangedBookkeeping(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	spec := ticketSpec(baseFields()...)
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{spec}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	const fixed = "2020-01-01 00:00:00+00:00"
	if err := db.Exec(`UPDATE `+ontology.AppliedSpecTable+` SET applied_at = ?`, fixed).Error; err != nil {
		t.Fatalf("pin applied_at: %v", err)
	}
	// Read it back through the same path the assertion uses: the driver renders
	// the stored timestamp its own way (sqlite returns RFC3339), and comparing
	// against the literal would test the driver, not the skip.
	readAppliedAt := func() string {
		t.Helper()
		var got string
		if err := db.Table(ontology.AppliedSpecTable).Select("applied_at").Where("slug = ?", "tickets").Row().Scan(&got); err != nil {
			t.Fatalf("read applied_at: %v", err)
		}
		return got
	}
	before := readAppliedAt()

	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{spec}); err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	if got := readAppliedAt(); got != before {
		t.Fatalf("applied_at = %q, want %q: an unchanged declaration must not touch the row", got, before)
	}
}

func TestPlanLeavesDatabaseUntouchedWhenDeclarationIsInvalid(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(baseFields()...)}); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	fields := baseFields()
	fields[1].Type = ontology.TypeInteger
	err := ontology.Apply(ctx, db, "sqlite", []ontology.Spec{ticketSpec(fields...)})
	if err == nil {
		t.Fatal("expected an error")
	}
	var hash string
	if err := db.Table(ontology.AppliedSpecTable).Select("hash").Where("slug = ?", "tickets").Row().Scan(&hash); err != nil {
		t.Fatalf("read bookkeeping: %v", err)
	}
	if hash == "" {
		t.Fatal("bookkeeping row lost")
	}
	// The stored declaration must still describe v1, not the refused v2.
	var stored string
	if err := db.Table(ontology.AppliedSpecTable).Select("spec_json").Where("slug = ?", "tickets").Row().Scan(&stored); err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var applied ontology.Spec
	if err := json.Unmarshal([]byte(stored), &applied); err != nil {
		t.Fatalf("decode stored spec: %v", err)
	}
	if applied.Fields[1].Type != ontology.TypeString {
		t.Fatalf("stored status type = %q, want string (v1 must be intact)", applied.Fields[1].Type)
	}
}

func assertIssues(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", want)
	}
	issues, ok := ontology.IsInvalid(err)
	if !ok {
		t.Fatalf("err = %v, want validation issues", err)
	}
	for _, iss := range issues {
		if iss.Code == want {
			return
		}
	}
	t.Fatalf("issues = %+v, want code %s", issues, want)
}

func ptr(s string) *string { return &s }
