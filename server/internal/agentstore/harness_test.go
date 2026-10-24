package agentstore

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/models"
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
	if err := db.AutoMigrate(&models.AgentHarnessSpec{}, &models.AgentHarnessLesson{}); err != nil {
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

func TestStage_StoresADraftThatIsNotLive(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if row.OrgID != 7 {
		t.Errorf("org = %d, want 7", row.OrgID)
	}
	if row.Version != 1 {
		t.Errorf("version = %d, want 1", row.Version)
	}
	if row.Status != models.AgentHarnessStatusDraft {
		t.Errorf("status = %q, want draft", row.Status)
	}
	if row.PublishedAt != nil {
		t.Error("a draft has no publish time")
	}
	if row.Fingerprint == "" {
		t.Error("the fingerprint is empty")
	}

	// A staged draft is not the workspace's harness.
	if _, err := store.Active(ctx, 7); err != ErrNotFound {
		t.Errorf("a draft must not be live, got %v", err)
	}
	spec, err := store.Spec(ctx, 7)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if spec.Preamble != harness.DefaultHarnessSpec().Preamble {
		t.Errorf("spec = %+v, want the built-in default", spec)
	}
}

func TestStage_RefusesAnInvalidSpecAndWritesNothing(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	invalid := testSpec()
	invalid.Version = 99
	invalid.Name = ""

	if _, err := store.Stage(ctx, 7, invalid, 42); err == nil {
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

func TestActivate_MakesAStageTheLiveVersion(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	live, err := store.Activate(ctx, 7, row.ID, 42)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if live.Status != models.AgentHarnessStatusPublished {
		t.Errorf("status = %q, want published", live.Status)
	}
	if live.PublishedAt == nil {
		t.Error("the publish time is not recorded")
	}

	active, err := store.Active(ctx, 7)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.ID != row.ID {
		t.Errorf("active version = %d, want %d", active.ID, row.ID)
	}

	spec, err := store.Spec(ctx, 7)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if spec.Preamble != testSpec().Preamble {
		t.Errorf("spec = %+v, want the activated one", spec)
	}
}

func TestActivate_ArchivesTheVersionThatWasLive(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Activate(ctx, 7, first.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	second := testSpec()
	second.Preamble = "You are the sales assistant."
	secondRow, err := store.Stage(ctx, 7, second, 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if secondRow.Version != 2 {
		t.Errorf("version = %d, want 2", secondRow.Version)
	}
	if _, err := store.Activate(ctx, 7, secondRow.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
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

// One workspace runs one harness: activating a version of another name must
// archive the version that was live, whatever it was called.
func TestActivate_ArchivesTheLiveVersionAcrossNames(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	support := testSpec()
	support.Name = "support"
	supportRow, err := store.Stage(ctx, 7, support, 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Activate(ctx, 7, supportRow.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	ops := testSpec()
	ops.Name = "ops"
	opsRow, err := store.Stage(ctx, 7, ops, 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Activate(ctx, 7, opsRow.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	var live int64
	if err := store.db.Model(&models.AgentHarnessSpec{}).
		Where("owner_id = ? AND status = ?", 7, models.AgentHarnessStatusPublished).
		Count(&live).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if live != 1 {
		t.Errorf("the workspace has %d live versions, want exactly 1", live)
	}
}

func TestActivate_ReactivatesAnArchivedVersion(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Activate(ctx, 7, first.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	second := testSpec()
	second.Preamble = "You are the sales assistant."
	secondRow, err := store.Stage(ctx, 7, second, 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Activate(ctx, 7, secondRow.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Rolling back to the archived version is just activating it again.
	rolled, err := store.Activate(ctx, 7, first.ID, 42)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if rolled.Status != models.AgentHarnessStatusPublished {
		t.Errorf("status = %q, want published", rolled.Status)
	}

	active, err := store.Active(ctx, 7)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.ID != first.ID {
		t.Errorf("active version = %d, want %d", active.ID, first.ID)
	}
}

func TestActivate_RefusesAVersionOfAnotherWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if _, err := store.Activate(ctx, 8, row.ID, 42); err == nil {
		t.Fatal("expected the cross-workspace activation to be refused")
	}
}

func TestDiscard_ThrowsADraftAway(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	discarded, err := store.Discard(ctx, 7, row.ID)
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if discarded.Status != models.AgentHarnessStatusDiscarded {
		t.Errorf("status = %q, want discarded", discarded.Status)
	}
}

func TestDiscard_RefusesAVersionThatWasLive(t *testing.T) {
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
		t.Fatal("expected the live version to be protected from being discarded")
	}
}

func TestList_ReturnsEveryVersionNewestFirst(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		spec := testSpec()
		spec.Preamble = "preamble " + string(rune('a'+i))
		if _, err := store.Stage(ctx, 7, spec, 42); err != nil {
			t.Fatalf("Stage: %v", err)
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

	if _, err := store.Stage(ctx, 7, testSpec(), 42); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	rows, err := store.List(ctx, 8)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("another workspace sees %d versions", len(rows))
	}
}

func TestSpec_FallsBackToTheBuiltInDefaultWhenNothingIsLive(t *testing.T) {
	store, _ := newTestStore(t)

	spec, err := store.Spec(context.Background(), 7)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if spec.Name != harness.DefaultHarnessSpec().Name {
		t.Errorf("spec = %+v, want the built-in default", spec)
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

func TestImportProposal_KeepsTheEvidence(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	evidence := json.RawMessage(`{"taskSet":"ops-tasks","candidate":"candidate-optimized"}`)
	row, err := store.ImportProposal(ctx, 7, testSpec(), evidence, 42)
	if err != nil {
		t.Fatalf("ImportProposal: %v", err)
	}
	if row.Status != models.AgentHarnessStatusDraft {
		t.Errorf("status = %q, want draft", row.Status)
	}
	if row.Evidence == "" {
		t.Fatal("the evidence was dropped")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(row.Evidence), &got); err != nil {
		t.Fatalf("the stored evidence is not JSON: %v", err)
	}
	if got["candidate"] != "candidate-optimized" {
		t.Errorf("evidence = %v", got)
	}
}

func TestCreateLesson_RecordsTheLesson(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.CreateLesson(ctx, 7, "links", "Always answer in the operator's language.", nil, 0, 42)
	if err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}
	if row.Status != models.AgentHarnessLessonSuggested {
		t.Errorf("status = %q, want suggested", row.Status)
	}
	if row.OrgID != 7 {
		t.Errorf("org = %d, want 7", row.OrgID)
	}
}

func TestCreateLesson_RefusesAnEmptyLesson(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if _, err := store.CreateLesson(ctx, 7, "", "note", nil, 0, 42); err == nil {
		t.Error("expected a lesson without a scope to be refused")
	}
	if _, err := store.CreateLesson(ctx, 7, "scope", "", nil, 0, 42); err == nil {
		t.Error("expected a lesson without a note to be refused")
	}
}

func TestListLessons_DoesNotLeakAnotherWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if _, err := store.CreateLesson(ctx, 7, "links", "note", nil, 0, 42); err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}
	rows, err := store.ListLessons(ctx, 8)
	if err != nil {
		t.Fatalf("ListLessons: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("another workspace sees %d lessons", len(rows))
	}
}

func TestDismissLesson_KeepsTheRecord(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.CreateLesson(ctx, 7, "links", "note", nil, 0, 42)
	if err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}

	dismissed, err := store.DismissLesson(ctx, 7, row.ID)
	if err != nil {
		t.Fatalf("DismissLesson: %v", err)
	}
	if dismissed.Status != models.AgentHarnessLessonDismissed {
		t.Errorf("status = %q, want dismissed", dismissed.Status)
	}

	rows, err := store.ListLessons(ctx, 7)
	if err != nil {
		t.Fatalf("ListLessons: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("the dismissed lesson disappeared: %d rows", len(rows))
	}
}

// Activating a version adopts the lesson that motivated it: the loop from
// "something was learned" to "it is now live" closes.
func TestActivate_AdoptsTheLessonThatMotivatedTheVersion(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	lesson, err := store.CreateLesson(ctx, 7, "harness", "Proposal imported: default", nil, row.ID, 42)
	if err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}

	if _, err := store.Activate(ctx, 7, row.ID, 42); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	rows, err := store.ListLessons(ctx, 7)
	if err != nil {
		t.Fatalf("ListLessons: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 lesson, got %d", len(rows))
	}
	if rows[0].ID != lesson.ID {
		t.Errorf("lesson = %d, want %d", rows[0].ID, lesson.ID)
	}
	if rows[0].Status != models.AgentHarnessLessonAdopted {
		t.Errorf("status = %q, want adopted", rows[0].Status)
	}
}

// Discarding the draft a lesson motivated dismisses the lesson with it.
func TestDiscard_DismissesTheLessonOfTheDiscardedDraft(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	row, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.CreateLesson(ctx, 7, "harness", "Proposal imported: default", nil, row.ID, 42); err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}

	if _, err := store.Discard(ctx, 7, row.ID); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	rows, err := store.ListLessons(ctx, 7)
	if err != nil {
		t.Fatalf("ListLessons: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 lesson, got %d", len(rows))
	}
	if rows[0].Status != models.AgentHarnessLessonDismissed {
		t.Errorf("status = %q, want dismissed", rows[0].Status)
	}
}

// The version counter is per workspace: two workspaces both start at 1.
func TestStage_VersionNumbersArePerWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, err := store.Stage(ctx, 7, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	other, err := store.Stage(ctx, 8, testSpec(), 42)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if first.Version != 1 || other.Version != 1 {
		t.Errorf("versions = %d and %d, want 1 and 1", first.Version, other.Version)
	}
}

// Two admins publishing at the same moment must not produce two versions
// with the same number: the pair (workspace, version) is unique.
func TestStage_RefusesADuplicateVersionNumber(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Stage(ctx, 7, testSpec(), 42); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	duplicate := &models.AgentHarnessSpec{
		OrgID:       7,
		Name:        "default",
		Version:     1,
		SpecJSON:    testSpec().CanonicalJSON(),
		Fingerprint: testSpec().Fingerprint(),
		Status:      models.AgentHarnessStatusDraft,
		CreatedBy:   42,
	}
	if err := db.Create(duplicate).Error; err == nil {
		t.Fatal("expected the duplicate version number to be refused")
	}
}
