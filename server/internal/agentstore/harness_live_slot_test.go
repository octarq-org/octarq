package agentstore

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

// The workspace runs one harness. The store's transaction keeps that true
// inside one process; the database keeps it true when two activations race.
// These tests check the second half: the guarantee has to survive a write
// that did not come through the store.

// A second live version is refused by the database, not by the store's
// bookkeeping: the unique index on (owner_id, live_slot) is the rule.
func TestActivate_TheDatabaseRefusesASecondLiveVersion(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	first, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	second, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if _, err := store.Activate(ctx, 7, first.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Publish the second version behind the store's back — what a racing
	// activation would do.
	err = db.Model(&models.AgentHarnessSpec{}).
		Where("id = ?", second.ID).
		Updates(map[string]any{
			"status":    models.AgentHarnessStatusPublished,
			"live_slot": 1,
		}).Error
	if err == nil {
		t.Fatal("the database accepted a second live version for one workspace")
	}
}

// The live version holds the slot, and no other row does.
func TestActivate_TheLiveVersionHoldsTheSlot(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	first, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	second, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if _, err := store.Activate(ctx, 7, first.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := store.Activate(ctx, 7, second.ID, 42); err != nil {
		t.Fatalf("Activate the second version: %v", err)
	}

	var live []models.AgentHarnessSpec
	if err := db.Where("owner_id = ? AND status = ?", 7, models.AgentHarnessStatusPublished).Find(&live).Error; err != nil {
		t.Fatalf("read the live versions: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("expected exactly one live version, got %d", len(live))
	}
	if live[0].ID != second.ID {
		t.Errorf("the live version is #%d, want #%d", live[0].ID, second.ID)
	}
	if live[0].LiveSlot == nil || *live[0].LiveSlot != 1 {
		t.Errorf("the live version does not hold the slot: %v", live[0].LiveSlot)
	}

	var holders int64
	if err := db.Model(&models.AgentHarnessSpec{}).
		Where("live_slot = 1").Count(&holders).Error; err != nil {
		t.Fatalf("count the slot holders: %v", err)
	}
	if holders != 1 {
		t.Errorf("%d rows hold the live slot, want 1", holders)
	}

	// The archived version released the slot.
	var archived models.AgentHarnessSpec
	if err := db.Where("id = ?", first.ID).First(&archived).Error; err != nil {
		t.Fatalf("read the archived version: %v", err)
	}
	if archived.Status != models.AgentHarnessStatusArchived {
		t.Errorf("status = %q, want archived", archived.Status)
	}
	if archived.LiveSlot != nil {
		t.Errorf("the archived version still holds the slot: %v", *archived.LiveSlot)
	}
}

// Two workspaces may each have a live version: the slot is per workspace.
func TestActivate_TwoWorkspacesMayEachHaveALiveVersion(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	first, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	other, err := store.Stage(ctx, 9, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if _, err := store.Activate(ctx, 7, first.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := store.Activate(ctx, 9, other.ID, 42); err != nil {
		t.Fatalf("Activate in the other workspace: %v", err)
	}

	var holders int64
	if err := db.Model(&models.AgentHarnessSpec{}).Where("live_slot = 1").Count(&holders).Error; err != nil {
		t.Fatalf("count the slot holders: %v", err)
	}
	if holders != 2 {
		t.Errorf("%d rows hold the live slot, want 2 — one per workspace", holders)
	}
}
