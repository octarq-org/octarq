package ontology

import (
	"context"
	"errors"
	"strings"
	"testing"

	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// writerHarness boots a plugin whose only entity is declared in Go and returns
// the writer the plugin published, so the test exercises the seam a module
// actually looks up rather than the store underneath it.
func writerHarness(t *testing.T, specs ...core.Spec) (core.RecordWriter, *gorm.DB) {
	t.Helper()
	return writerHarnessOn(t, true, specs...)
}

// writerHarnessOn is writerHarness with the workspace's switch for the declaring
// plugin set explicitly.
func writerHarnessOn(t *testing.T, on bool, specs ...core.Spec) (core.RecordWriter, *gorm.DB) {
	t.Helper()
	p := registryPlugin(t, specs...)
	db := openBookkeepingDB(t)
	var got core.RecordWriter
	ctx := &plugin.Context{
		DB: db,
		Host: &plugin.TestHost{TenantDBMock: func(orgID uint) *plugin.TenantDB {
			tdb, err := plugin.NewTenantDB(db, orgID)
			if err != nil {
				return nil
			}
			return tdb
		}},
		ActivePlugins: func() []plugin.Plugin { return []plugin.Plugin{notesOwner{}} },
		FeatureActive: func(uint, string) bool { return on },
		Provide: func(name string, svc any) {
			if name != core.ServiceRecords {
				return
			}
			got, _ = svc.(core.RecordWriter)
		},
	}
	p.Mount(nil, ctx)
	if p.err != nil {
		t.Fatalf("mount: %v", p.err)
	}
	if got == nil {
		t.Fatal("the plugin published no record writer")
	}
	return got, db
}

// TestTheWriterRoundTripsARow pins the module path end to end: a declaration in
// Go, a row written through the service, and the row read back in the shape the
// REST surface renders.
func TestTheWriterRoundTripsARow(t *testing.T) {
	w, _ := writerHarness(t, codeSpec())
	ctx := context.Background()

	created, err := w.Create(ctx, 1, "notes", map[string]any{"body": "The deploy is done."})
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	id, _ := created[core.ColumnID].(string)
	if id == "" {
		t.Fatalf("created row has no id: %v", created)
	}
	if created["body"] != "The deploy is done." {
		t.Fatalf("body = %v", created["body"])
	}

	read, err := w.Get(ctx, 1, "notes", id)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if read["body"] != created["body"] {
		t.Fatalf("Get body = %v, want %v", read["body"], created["body"])
	}
	if _, ok := read[core.ColumnOrgID]; ok {
		t.Fatal("the row exposes its workspace; FromRow must not")
	}
}

// TestTheWriterRefusesKeysTheDeclarationDoesNotOwn pins that the declaration
// still owns the column set: a body key cannot become a column.
func TestTheWriterRefusesKeysTheDeclarationDoesNotOwn(t *testing.T) {
	w, _ := writerHarness(t, codeSpec())
	ctx := context.Background()

	_, err := w.Create(ctx, 1, "notes", map[string]any{"body": "x", core.ColumnOrgID: 2})
	if err == nil || !strings.Contains(err.Error(), "managed by the platform") {
		t.Fatalf("reserved key: err = %v, want a platform-key refusal", err)
	}
	_, err = w.Create(ctx, 1, "notes", map[string]any{"body": "x", "nope": 1})
	if err == nil || !strings.Contains(err.Error(), "not a declared field") {
		t.Fatalf("unknown key: err = %v, want an undeclared-field refusal", err)
	}
	_, err = w.Create(ctx, 1, "notes", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("missing required: err = %v, want a required-field refusal", err)
	}
	_, err = w.Create(ctx, 1, "no_such_entity", map[string]any{"body": "x"})
	if err == nil || !strings.Contains(err.Error(), "no entity is declared") {
		t.Fatalf("unknown slug: err = %v, want an unknown-entity refusal", err)
	}
}

// TestTheWriterKeepsOtherWorkspacesOut pins the tenant predicate on every
// method, including Find.
func TestTheWriterKeepsOtherWorkspacesOut(t *testing.T) {
	w, _ := writerHarness(t, codeSpec())
	ctx := context.Background()

	created, err := w.Create(ctx, 1, "notes", map[string]any{"body": "only mine"})
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	id, _ := created[core.ColumnID].(string)

	if _, err := w.Get(ctx, 2, "notes", id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get as another workspace = %v, want ErrNotFound", err)
	}
	rows, err := w.Find(ctx, 2, "notes", "body", "only mine")
	if err != nil {
		t.Fatalf("Find as another workspace = %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("another workspace sees %d rows, want none", len(rows))
	}
	if _, err := w.Update(ctx, 2, "notes", id, map[string]any{"body": "taken"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update as another workspace = %v, want ErrNotFound", err)
	}
}

// TestTheWriterFindsByADeclaredField pins that the lookup column is checked
// against the declaration before SQL is built.
func TestTheWriterFindsByADeclaredField(t *testing.T) {
	w, _ := writerHarness(t, codeSpec())
	ctx := context.Background()

	for _, body := range []string{"first", "second", "second"} {
		if _, err := w.Create(ctx, 1, "notes", map[string]any{"body": body}); err != nil {
			t.Fatalf("Create(%q) = %v", body, err)
		}
	}
	rows, err := w.Find(ctx, 1, "notes", "body", "second")
	if err != nil {
		t.Fatalf("Find = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("Find = %d rows, want 2", len(rows))
	}
	for _, field := range []string{"nope", core.ColumnOrgID, core.ColumnUserID} {
		if _, err := w.Find(ctx, 1, "notes", field, "second"); err == nil {
			t.Fatalf("Find(%q) = nil error, want a refusal", field)
		}
	}
}

// TestAUserScopedEntityBindsTheMember pins the member-scope contract: a call
// with no member is refused rather than widened to the workspace, and one
// member cannot read another's rows.
func TestAUserScopedEntityBindsTheMember(t *testing.T) {
	spec := codeSpec()
	spec.Slug = "mynotes"
	spec.Scope = core.ScopeUser
	w, _ := writerHarness(t, spec)

	if _, err := w.Create(context.Background(), 1, "mynotes", map[string]any{"body": "x"}); !errors.Is(err, ErrNoUserScope) {
		t.Fatalf("Create without a member = %v, want ErrNoUserScope", err)
	}

	mine := plugin.WithUserID(context.Background(), 7)
	created, err := w.Create(mine, 1, "mynotes", map[string]any{"body": "mine"})
	if err != nil {
		t.Fatalf("Create as a member = %v", err)
	}
	if created["body"] != "mine" {
		t.Fatalf("body = %v", created["body"])
	}

	theirs := plugin.WithUserID(context.Background(), 8)
	rows, err := w.Find(theirs, 1, "mynotes", "body", "mine")
	if err != nil {
		t.Fatalf("Find as another member = %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("another member sees %d rows, want none", len(rows))
	}
}

// TestTheWriterRefusesAnEntityWhosePluginIsOff pins the switch on the module
// write seam. A module that keeps writing through the service while its page is
// gone would be a second, invisible switch.
func TestTheWriterRefusesAnEntityWhosePluginIsOff(t *testing.T) {
	w, _ := writerHarnessOn(t, false, codeSpec())
	ctx := context.Background()

	if _, err := w.Create(ctx, 1, "notes", map[string]any{"body": "x"}); !errors.Is(err, ErrFeatureOff) {
		t.Fatalf("Create while the plugin is off = %v, want ErrFeatureOff", err)
	}
	if _, err := w.Get(ctx, 1, "notes", "some-id"); !errors.Is(err, ErrFeatureOff) {
		t.Fatalf("Get while the plugin is off = %v, want ErrFeatureOff", err)
	}
	if _, err := w.Update(ctx, 1, "notes", "some-id", map[string]any{"body": "y"}); !errors.Is(err, ErrFeatureOff) {
		t.Fatalf("Update while the plugin is off = %v, want ErrFeatureOff", err)
	}
	if _, err := w.Find(ctx, 1, "notes", "body", "x"); !errors.Is(err, ErrFeatureOff) {
		t.Fatalf("Find while the plugin is off = %v, want ErrFeatureOff", err)
	}
}
