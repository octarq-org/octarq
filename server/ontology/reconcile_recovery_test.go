package ontology

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openRecoveryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&AppliedSpec{}); err != nil {
		t.Fatalf("migrate bookkeeping: %v", err)
	}
	return db
}

func TestApplyStepClaimsTableCreatedBeforeAFailure(t *testing.T) {
	db := openRecoveryDB(t)
	ctx := context.Background()
	spec := Spec{
		Slug:       "tickets",
		Label:      "Ticket",
		Fields:     []Field{{Name: "title", Type: TypeString, Required: true, Unique: true}},
		Operations: AllowAllOperations(),
	}
	steps, err := Plan(ctx, db, "sqlite", []Spec{spec})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	step := steps[0]

	step.Statements = append(
		[]string{step.Statements[0], `ALTER TABLE "ont_tickets" ADD COLUMN "id" TEXT`},
		step.Statements[1:]...)
	if err := applyStep(ctx, db, step); err == nil {
		t.Fatal("expected the injected statement to fail")
	}

	var n int64
	if err := db.Table(AppliedSpecTable).Where("slug = ?", "tickets").Count(&n).Error; err != nil {
		t.Fatalf("count bookkeeping rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("bookkeeping rows = %d, want the created table claimed", n)
	}

	if err := Apply(ctx, db, "sqlite", []Spec{spec}); err != nil {
		t.Fatalf("recovery Apply: %v", err)
	}
	insp, err := InspectorFor("sqlite", db)
	if err != nil {
		t.Fatalf("InspectorFor: %v", err)
	}
	idx, err := insp.Indexes(ctx, "ont_tickets")
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	for _, want := range []string{
		TenantIndexName(spec),
		FieldIndexName(spec, spec.Fields[0]),
	} {
		if !idx[want] {
			t.Fatalf("index %q was not repaired on the next boot: %v", want, idx)
		}
	}
}

func TestFailedChangeKeepsTheAppliedDeclaration(t *testing.T) {
	db := openRecoveryDB(t)
	ctx := context.Background()
	v1 := Spec{
		Slug:       "tickets",
		Label:      "Ticket",
		Fields:     []Field{{Name: "title", Type: TypeString, Required: true}},
		Operations: AllowAllOperations(),
	}
	if err := Apply(ctx, db, "sqlite", []Spec{v1}); err != nil {
		t.Fatalf("apply v1: %v", err)
	}

	v2 := v1
	v2.Fields = append(append([]Field{}, v1.Fields...), Field{Name: "code", Type: TypeString, Unique: true})
	steps, err := Plan(ctx, db, "sqlite", []Spec{v2})
	if err != nil {
		t.Fatalf("Plan v2: %v", err)
	}
	step := steps[0]
	if !step.Dirty || !step.existed {
		t.Fatalf("step = dirty %v, existed %v, want a change to an existing table", step.Dirty, step.existed)
	}
	step.Statements = append(step.Statements, `ALTER TABLE "ont_tickets" ADD COLUMN "id" TEXT`)
	if err := applyStep(ctx, db, step); err == nil {
		t.Fatal("expected the injected statement to fail")
	}

	var applied AppliedSpec
	if err := db.Table(AppliedSpecTable).Where("slug = ?", "tickets").First(&applied).Error; err != nil {
		t.Fatalf("read bookkeeping row: %v", err)
	}
	var stored Spec
	if err := json.Unmarshal([]byte(applied.SpecJSON), &stored); err != nil {
		t.Fatalf("decode stored spec: %v", err)
	}
	if len(stored.Fields) != len(v1.Fields) {
		t.Fatalf("stored declaration has %d fields, want the applied v1 with %d", len(stored.Fields), len(v1.Fields))
	}
	v1Hash, err := v1.Hash()
	if err != nil {
		t.Fatalf("hash v1: %v", err)
	}
	if applied.Hash != v1Hash {
		t.Fatalf("stored hash = %s, want v1's %s", applied.Hash, v1Hash)
	}
}
