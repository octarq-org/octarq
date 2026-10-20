package app

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/models"
)

// The host is how a plugin's agent learns the workspace's published harness.
// Without it every channel would run on a prompt of its own.

// bootHarnessApp boots the app and migrates the harness tables. New() opens
// the database but does not migrate it — the migration pass belongs to Run().
func bootHarnessApp(t *testing.T) *App {
	t.Helper()
	a := bootApp(t)
	if err := a.gdb.AutoMigrate(&models.AgentHarnessSpec{}, &models.AgentHarnessLesson{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return a
}

func TestHostRuntime_HarnessSpecIsEmptyWhenNothingIsPublished(t *testing.T) {
	a := bootHarnessApp(t)
	h := &hostRuntime{harnessStore: agentstore.NewHarnessStore(a.gdb)}

	raw, err := h.HarnessSpec(context.Background(), 1)
	if err != nil {
		t.Fatalf("HarnessSpec: %v", err)
	}
	if len(raw) != 0 {
		t.Errorf("expected no published harness, got %s", raw)
	}
}

// A staged draft is not the workspace's harness: the host must not hand it
// out before somebody activates it.
func TestHostRuntime_HarnessSpecIgnoresAStagedDraft(t *testing.T) {
	a := bootHarnessApp(t)
	store := agentstore.NewHarnessStore(a.gdb)
	h := &hostRuntime{harnessStore: store}
	ctx := context.Background()

	spec := harness.HarnessSpec{Version: harness.HarnessSpecVersion, Name: "default", Preamble: "draft"}
	if _, err := store.Stage(ctx, 1, spec, 1); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	raw, err := h.HarnessSpec(ctx, 1)
	if err != nil {
		t.Fatalf("HarnessSpec: %v", err)
	}
	if len(raw) != 0 {
		t.Errorf("a draft was handed out as the live harness: %s", raw)
	}
}

func TestHostRuntime_HarnessSpecReturnsThePublishedDocument(t *testing.T) {
	a := bootHarnessApp(t)
	store := agentstore.NewHarnessStore(a.gdb)
	h := &hostRuntime{harnessStore: store}
	ctx := context.Background()

	spec := harness.HarnessSpec{
		Version:  harness.HarnessSpecVersion,
		Name:     "default",
		Preamble: "You are the support agent of this workspace.",
	}
	row, err := store.Stage(ctx, 1, spec, 1)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Activate(ctx, 1, row.ID, 1); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	raw, err := h.HarnessSpec(ctx, 1)
	if err != nil {
		t.Fatalf("HarnessSpec: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("the published harness is empty")
	}

	// The document must decode with the harness package's own decoder —
	// that is the contract the plugin side relies on.
	got, err := harness.DecodeSpec(raw)
	if err != nil {
		t.Fatalf("DecodeSpec: %v (%s)", err, raw)
	}
	if got.Preamble != spec.Preamble {
		t.Errorf("preamble = %q, want %q", got.Preamble, spec.Preamble)
	}
}

func TestHostRuntime_HarnessSpecIsTenantScoped(t *testing.T) {
	a := bootHarnessApp(t)
	store := agentstore.NewHarnessStore(a.gdb)
	h := &hostRuntime{harnessStore: store}
	ctx := context.Background()

	spec := harness.HarnessSpec{Version: harness.HarnessSpecVersion, Name: "default"}
	row, err := store.Stage(ctx, 1, spec, 1)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := store.Activate(ctx, 1, row.ID, 1); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	other, err := h.HarnessSpec(ctx, 2)
	if err != nil {
		t.Fatalf("HarnessSpec: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("another workspace sees a harness: %s", other)
	}
}

// A published spec that cannot be decoded must be reported, not hidden: the
// caller has to fail closed rather than run a different harness.
func TestHostRuntime_HarnessSpecReportsABrokenDocument(t *testing.T) {
	a := bootHarnessApp(t)
	store := agentstore.NewHarnessStore(a.gdb)
	h := &hostRuntime{harnessStore: store}
	ctx := context.Background()

	if err := a.gdb.Create(&models.AgentHarnessSpec{
		OrgID:       1,
		Name:        "default",
		Version:     1,
		SpecJSON:    `{"version":1,"name":"default","skills":["do-thing"]}`,
		Fingerprint: "abc",
		Status:      models.AgentHarnessStatusPublished,
		CreatedBy:   1,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	raw, err := h.HarnessSpec(ctx, 1)
	if err != nil {
		t.Fatalf("HarnessSpec: %v", err)
	}
	if _, err := harness.DecodeSpec(raw); err == nil {
		t.Fatal("expected the broken document to be refused by the decoder")
	}
}

func TestHostRuntime_HarnessSpecWithoutAStoreIsEmpty(t *testing.T) {
	h := &hostRuntime{}

	raw, err := h.HarnessSpec(context.Background(), 1)
	if err != nil {
		t.Fatalf("HarnessSpec: %v", err)
	}
	if len(raw) != 0 {
		t.Errorf("expected no harness, got %s", raw)
	}
}
