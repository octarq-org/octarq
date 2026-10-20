package agentstore

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestStore(t *testing.T) (*HarnessStore, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AgentHarnessSpec{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return NewHarnessStore(db), db
}

func testSpec() harness.HarnessSpec {
	return harness.HarnessSpec{
		Version:  harness.HarnessSpecVersion,
		Name:     "default",
		Preamble: "You are the workspace assistant.",
	}
}

func TestPublish_StoresTheSpecAndMakesItActive(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Publish(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if row.OrgID != 7 {
		t.Errorf("org = %d, want 7", row.OrgID)
	}
	if row.Version != 1 {
		t.Errorf("version = %d, want 1", row.Version)
	}
	if row.Status != models.AgentHarnessStatusPublished {
		t.Errorf("status = %q, want published", row.Status)
	}
	if row.Fingerprint == "" {
		t.Error("the fingerprint is empty")
	}
	if row.PublishedAt == nil {
		t.Error("the publish time is not recorded")
	}

	active, err := store.Active(ctx, 7)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.ID != row.ID {
		t.Errorf("active version = %d, want %d", active.ID, row.ID)
	}
}

func TestPublish_RefusesAnInvalidSpecAndWritesNothing(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	invalid := testSpec()
	invalid.Version = 99
	invalid.Name = ""

	if _, err := store.Publish(ctx, 7, invalid, 42); err == nil {
		t.Fatal("expected the invalid spec to be refused")
	}

	var count int64
	if err := db.Model(&models.AgentHarnessSpec{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("a refused spec wrote %d rows", count)
	}
}

func TestPublish_ArchivesThePreviousVersion(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, err := store.Publish(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	second := testSpec()
	second.Preamble = "You are the sales assistant."
	secondRow, err := store.Publish(ctx, 7, second, 42)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if secondRow.Version != 2 {
		t.Errorf("version = %d, want 2", secondRow.Version)
	}

	archived, err := store.Get(ctx, 7, first.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if archived.Status != models.AgentHarnessStatusArchived {
		t.Errorf("the previous version is %q, want archived", archived.Status)
	}

	active, err := store.Active(ctx, 7)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.ID != secondRow.ID {
		t.Errorf("active version = %d, want %d", active.ID, secondRow.ID)
	}
}

func TestRollback_MakesAnArchivedVersionActiveAgain(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, err := store.Publish(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	second := testSpec()
	second.Preamble = "You are the sales assistant."
	secondRow, err := store.Publish(ctx, 7, second, 42)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	rolled, err := store.Rollback(ctx, 7, first.ID, 42)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rolled.Status != models.AgentHarnessStatusPublished {
		t.Errorf("the rolled back version is %q, want published", rolled.Status)
	}

	active, err := store.Active(ctx, 7)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.ID != first.ID {
		t.Errorf("active version = %d, want %d", active.ID, first.ID)
	}

	other, err := store.Get(ctx, 7, secondRow.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if other.Status != models.AgentHarnessStatusArchived {
		t.Errorf("the replaced version is %q, want archived", other.Status)
	}
}

func TestRollback_RefusesAVersionOfAnotherWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Publish(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if _, err := store.Rollback(ctx, 8, row.ID, 42); err == nil {
		t.Fatal("expected the cross-workspace rollback to be refused")
	}
}

func TestList_ReturnsEveryVersionNewestFirst(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		spec := testSpec()
		spec.Preamble = "preamble " + string(rune('a'+i))
		if _, err := store.Publish(ctx, 7, spec, 42); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	rows, err := store.List(ctx, 7)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(rows))
	}
	if rows[0].Version != 3 || rows[2].Version != 1 {
		t.Errorf("versions are not ordered newest first: %d, %d", rows[0].Version, rows[2].Version)
	}
}

func TestList_DoesNotLeakAnotherWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Publish(ctx, 7, testSpec(), 42); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	rows, err := store.List(ctx, 8)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("another workspace sees %d versions", len(rows))
	}
}

func TestSpec_FallsBackToTheBuiltInDefaultWhenNothingIsPublished(t *testing.T) {
	store, _ := newTestStore(t)

	spec, err := store.Spec(context.Background(), 7)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if spec.Name != harness.DefaultHarnessSpec().Name {
		t.Errorf("spec = %+v, want the built-in default", spec)
	}
}

func TestSpec_ReturnsThePublishedSpec(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	published := testSpec()
	published.Preamble = "You are the support agent."
	if _, err := store.Publish(ctx, 7, published, 42); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	spec, err := store.Spec(ctx, 7)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if spec.Preamble != published.Preamble {
		t.Errorf("preamble = %q, want %q", spec.Preamble, published.Preamble)
	}
}

func TestSpec_FailsClosedOnABrokenStoredSpec(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	// A row that cannot be decoded must not be silently replaced by the
	// default: that would hide a broken configuration.
	if err := db.Create(&models.AgentHarnessSpec{
		OrgID:       7,
		Name:        "default",
		Version:     1,
		SpecJSON:    `{"version":1,"name":"default","skills":["do-thing"]}`,
		Fingerprint: "abc",
		Status:      models.AgentHarnessStatusPublished,
		CreatedBy:   42,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := store.Spec(ctx, 7); err == nil {
		t.Fatal("expected a broken stored spec to be reported")
	}
}
