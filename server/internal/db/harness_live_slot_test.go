package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

// A workspace runs one harness. The database has to say so, and the rule has
// to survive an install that already broke it: before the live slot existed,
// two activations could race and leave two published rows behind.

func openLiveSlotDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "live-slot.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return gdb
}

// seedLegacyHarnessTable creates the shape the table had before the live slot
// existed: no live_slot column, no unique index.
func seedLegacyHarnessTable(t *testing.T, gdb *gorm.DB) {
	t.Helper()
	if err := gdb.Exec(`CREATE TABLE agent_harness_specs (
		id integer PRIMARY KEY AUTOINCREMENT,
		owner_id integer NOT NULL,
		name text NOT NULL,
		version integer NOT NULL,
		spec_json text NOT NULL,
		fingerprint text NOT NULL,
		status text NOT NULL,
		evidence text,
		created_by integer NOT NULL,
		created_at datetime,
		published_at datetime
	)`).Error; err != nil {
		t.Fatalf("create the legacy table: %v", err)
	}
	now := time.Now().UTC()
	rows := []map[string]any{
		// Workspace 1 published twice — the state the race could leave behind.
		{"owner_id": 1, "name": "first", "version": 1, "spec_json": "{}", "fingerprint": "fp1", "status": models.AgentHarnessStatusPublished, "created_by": 1, "created_at": now, "published_at": now},
		{"owner_id": 1, "name": "second", "version": 2, "spec_json": "{}", "fingerprint": "fp2", "status": models.AgentHarnessStatusPublished, "created_by": 1, "created_at": now, "published_at": now},
		// Workspace 2 published once.
		{"owner_id": 2, "name": "other", "version": 1, "spec_json": "{}", "fingerprint": "fp3", "status": models.AgentHarnessStatusPublished, "created_by": 1, "created_at": now, "published_at": now},
		// A draft, which holds no slot.
		{"owner_id": 1, "name": "draft", "version": 3, "spec_json": "{}", "fingerprint": "fp4", "status": models.AgentHarnessStatusDraft, "created_by": 1, "created_at": now},
	}
	for _, row := range rows {
		if err := gdb.Table("agent_harness_specs").Create(row).Error; err != nil {
			t.Fatalf("seed the harness versions: %v", err)
		}
	}
}

// The migration repairs a workspace that published twice and installs the
// index that keeps it from happening again.
func TestMigrate_RepairsADoublePublishedWorkspaceAndInstallsTheIndex(t *testing.T) {
	gdb := openLiveSlotDB(t)
	seedLegacyHarnessTable(t, gdb)

	if err := Migrate(gdb); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Workspace 1 keeps the newer version; the older one is archived.
	var live []models.AgentHarnessSpec
	if err := gdb.Where("owner_id = ? AND status = ?", 1, models.AgentHarnessStatusPublished).Find(&live).Error; err != nil {
		t.Fatalf("read the live versions: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("workspace 1 has %d live versions, want 1", len(live))
	}
	if live[0].Version != 2 {
		t.Errorf("the live version is #%d, want the newer #2", live[0].Version)
	}
	if live[0].LiveSlot == nil || *live[0].LiveSlot != 1 {
		t.Errorf("the live version does not hold the slot: %v", live[0].LiveSlot)
	}

	var archived int64
	if err := gdb.Model(&models.AgentHarnessSpec{}).
		Where("owner_id = ? AND status = ?", 1, models.AgentHarnessStatusArchived).Count(&archived).Error; err != nil {
		t.Fatalf("count the archived versions: %v", err)
	}
	if archived != 1 {
		t.Errorf("workspace 1 has %d archived versions, want 1", archived)
	}

	// Workspace 2 keeps its own live version: the slot is per workspace.
	var other []models.AgentHarnessSpec
	if err := gdb.Where("owner_id = ? AND status = ?", 2, models.AgentHarnessStatusPublished).Find(&other).Error; err != nil {
		t.Fatalf("read the other workspace's live versions: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("workspace 2 has %d live versions, want 1", len(other))
	}

	// The draft holds no slot.
	var draft models.AgentHarnessSpec
	if err := gdb.Where("owner_id = ? AND status = ?", 1, models.AgentHarnessStatusDraft).First(&draft).Error; err != nil {
		t.Fatalf("read the draft: %v", err)
	}
	if draft.LiveSlot != nil {
		t.Errorf("the draft holds the live slot: %v", *draft.LiveSlot)
	}

	// The index is installed, and it refuses a second live version.
	if !gdb.Migrator().HasIndex(&models.AgentHarnessSpec{}, "idx_agent_harness_live_slot") {
		t.Fatal("the live slot index was not installed")
	}
	err := gdb.Model(&models.AgentHarnessSpec{}).
		Where("owner_id = ?", 1).
		Where("status <> ?", models.AgentHarnessStatusPublished).
		Updates(map[string]any{"status": models.AgentHarnessStatusPublished, "live_slot": 1}).Error
	if err == nil {
		t.Fatal("the database accepted a second live version for one workspace")
	}
}

// The migration is idempotent: it runs on every start and must not change a
// healthy install.
func TestMigrate_IsIdempotentForTheLiveSlot(t *testing.T) {
	gdb := openLiveSlotDB(t)
	seedLegacyHarnessTable(t, gdb)

	if err := Migrate(gdb); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	var before int64
	if err := gdb.Model(&models.AgentHarnessSpec{}).Where("live_slot = 1").Count(&before).Error; err != nil {
		t.Fatalf("count the slot holders: %v", err)
	}

	if err := Migrate(gdb); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var after int64
	if err := gdb.Model(&models.AgentHarnessSpec{}).Where("live_slot = 1").Count(&after).Error; err != nil {
		t.Fatalf("count the slot holders: %v", err)
	}
	if before != after {
		t.Errorf("the slot holders changed from %d to %d", before, after)
	}
}
