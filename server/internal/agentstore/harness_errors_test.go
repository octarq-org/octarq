package agentstore

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/models"
)

// The store's refusals are part of its contract: a version of another
// workspace is invisible, a version that was thrown away cannot come back,
// and a store without a database reports "nothing published" rather than
// pretending to know.

func TestStore_WithoutADatabaseReportsNothingPublished(t *testing.T) {
	store := NewHarnessStore(nil)
	ctx := context.Background()

	if _, err := store.Active(ctx, 1); err != ErrNotFound {
		t.Errorf("Active = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(ctx, 1, 1); err != ErrNotFound {
		t.Errorf("Get = %v, want ErrNotFound", err)
	}
	rows, err := store.List(ctx, 1)
	if err != nil || len(rows) != 0 {
		t.Errorf("List = %v, %v, want no rows", rows, err)
	}
	lessons, err := store.ListLessons(ctx, 1)
	if err != nil || len(lessons) != 0 {
		t.Errorf("ListLessons = %v, %v, want no rows", lessons, err)
	}
	spec, err := store.Spec(ctx, 1)
	if err != nil {
		t.Errorf("Spec = %v, want the built-in default", err)
	}
	if spec.Name != harness.DefaultHarnessSpec().Name {
		t.Errorf("spec = %+v, want the built-in default", spec)
	}
	if _, err := store.Stage(ctx, 1, harness.DefaultHarnessSpec(), 1); err == nil {
		t.Error("expected staging without a database to be refused")
	}
	if _, err := store.CreateLesson(ctx, 1, "scope", "note", nil, 0, 1); err == nil {
		t.Error("expected recording a lesson without a database to be refused")
	}
}

func TestGet_RefusesAVersionOfAnotherWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Get(ctx, 8, row.ID); err != ErrNotFound {
		t.Errorf("Get = %v, want ErrNotFound", err)
	}
}

func TestActivate_RefusesADiscardedVersion(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Discard(ctx, 7, row.ID); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := store.Activate(ctx, 7, row.ID, 42); err == nil {
		t.Error("expected a discarded version to stay discarded")
	}
}

func TestActivate_RefusesAnUnknownVersion(t *testing.T) {
	store, _ := newTestStore(t)

	if _, err := store.Activate(context.Background(), 7, 4242, 42); err != ErrNotFound {
		t.Errorf("Activate = %v, want ErrNotFound", err)
	}
}

func TestDiscard_RefusesAVersionOfAnotherWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Discard(ctx, 8, row.ID); err != ErrNotFound {
		t.Errorf("Discard = %v, want ErrNotFound", err)
	}
}

func TestDiscard_RefusesAnArchivedVersion(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Activate(ctx, 7, row.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := store.Discard(ctx, 7, row.ID); err == nil {
		t.Error("expected an archived version to be kept")
	}
}

func TestDismissLesson_RefusesAnUnknownLesson(t *testing.T) {
	store, _ := newTestStore(t)

	if _, err := store.DismissLesson(context.Background(), 7, 4242); err != ErrNotFound {
		t.Errorf("DismissLesson = %v, want ErrNotFound", err)
	}
}

func TestImportProposal_RefusesAnInvalidSpec(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	invalid := testSpec()
	invalid.Version = 99
	if _, err := store.ImportProposal(ctx, 7, invalid, nil, 42); err == nil {
		t.Fatal("expected the invalid spec to be refused")
	}
	var count int64
	if err := db.Model(&models.AgentHarnessSpec{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("a refused proposal wrote %d rows", count)
	}
}
