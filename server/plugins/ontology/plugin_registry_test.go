package ontology

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// codeSpec is a valid declaration a module would register at init. It is
// deliberately not the acceptance entity: the store tests already own tickets.
func codeSpec() core.Spec {
	return core.Spec{
		Slug:  "notes",
		Label: "Note",
		Fields: []core.Field{
			{Name: "body", Type: core.TypeText, Required: true, ListColumn: true},
		},
		Operations: core.AllowAllOperations(),
	}
}

// notesOwner is the plugin that declares codeSpec, standing in for a real module.
// The composition must answer to the name the declaration carries, or the
// entity would have no switch and boot would refuse it.
type notesOwner struct{}

func (notesOwner) Name() string                      { return "notes" }
func (notesOwner) Describe() plugin.Info             { return plugin.Info{Title: "Notes"} }
func (notesOwner) Models() []any                     { return nil }
func (notesOwner) Mount(plugin.Mux, *plugin.Context) {}

// wiring is the context a composition root provides: the composed plugins, so a
// declaration's owner resolves to a feature, and the workspace's switch.
func wiring(db *gorm.DB, on bool) *plugin.Context {
	return &plugin.Context{
		DB:            db,
		ActivePlugins: func() []plugin.Plugin { return []plugin.Plugin{notesOwner{}} },
		FeatureActive: func(uint, string) bool { return on },
	}
}

// registryPlugin returns a plugin reading its own registry, with no declaration
// file in the environment.
func registryPlugin(t *testing.T, specs ...core.Spec) *Plugin {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv(EntitiesEnv, "")
	t.Setenv(EntitiesFileEnv, "")

	r := core.NewRegistry()
	if len(specs) > 0 {
		if err := r.Register("notes", specs...); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	return NewWithRegistry(r)
}

// openBookkeepingDB returns a database with the bookkeeping table and nothing
// else, so a code-declared entity is the only one it can hold.
func openBookkeepingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&core.AppliedSpec{}); err != nil {
		t.Fatalf("migrate bookkeeping: %v", err)
	}
	return db
}

// TestNewReadsTheProcessWideRegistry pins the wiring a module depends on: it
// registers into ontology.Default(), and builtin.Default()'s ontology.New() must
// be the reader. Nothing else carries the declarations across.
func TestNewReadsTheProcessWideRegistry(t *testing.T) {
	if got := New().registry; got != core.Default() {
		t.Fatal("New() does not read the process-wide registry")
	}
}

// TestCodeDeclaredEntityIsApplied pins the module path end to end at boot: a
// declaration that exists only in Go still produces a table and a sidebar entry.
func TestCodeDeclaredEntityIsApplied(t *testing.T) {
	p := registryPlugin(t, codeSpec())
	db := openBookkeepingDB(t)
	p.Mount(nil, wiring(db, true))

	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
	if !db.Migrator().HasTable(core.TableName("notes")) {
		t.Fatal("the code-declared table was not created")
	}
	menus := p.Menus()
	if len(menus) != 1 || menus[0].Path != "/data/notes" {
		t.Fatalf("menus = %v, want one entry for /data/notes", menus)
	}
}

// TestCodeAndFileDeclarationsAreMerged pins that the two sources are additive:
// an installation can declare entities and a build can ship more.
func TestCodeAndFileDeclarationsAreMerged(t *testing.T) {
	p := registryPlugin(t, codeSpec())
	t.Setenv(EntitiesEnv, ticketDoc)

	db := openBookkeepingDB(t)
	p.Mount(nil, wiring(db, true))

	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
	paths := make([]string, 0, 2)
	for _, m := range p.Menus() {
		paths = append(paths, m.Path)
	}
	if len(paths) != 2 || !contains(paths, "/data/tickets") || !contains(paths, "/data/notes") {
		t.Fatalf("menus = %v, want both the file and the code entity", paths)
	}
}

// TestASlugDeclaredInBothSourcesRefusesBoot pins the merge rule. Applying one of
// the two would leave the operator and the module believing different things
// about the same table.
func TestASlugDeclaredInBothSourcesRefusesBoot(t *testing.T) {
	spec := codeSpec()
	spec.Slug = "tickets"
	p := registryPlugin(t, spec)
	t.Setenv(EntitiesEnv, ticketDoc)

	p.Mount(nil, wiring(openBookkeepingDB(t), true))

	err := p.Validate(context.Background(), nil)
	if err == nil {
		t.Fatal("Validate = nil, want the duplicate slug refused")
	}
	if !strings.Contains(err.Error(), `"tickets" is declared both`) {
		t.Fatalf("Validate = %v, want a message naming the entity and both sources", err)
	}
}

// TestAMissingRegistryRefusesBoot pins fail-closed construction: without a
// registry the plugin cannot see any module's entities, and applying only the
// file would be half of what the binary declares.
func TestAMissingRegistryRefusesBoot(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EntitiesEnv, "")
	t.Setenv(EntitiesFileEnv, "")

	p := NewWithRegistry(nil)
	p.Mount(nil, wiring(openBookkeepingDB(t), true))

	if err := p.Validate(context.Background(), nil); !errors.Is(err, ErrNoRegistry) {
		t.Fatalf("Validate = %v, want ErrNoRegistry", err)
	}
}

// TestAnEmptyRegistryWithNoFileIsOff pins that a build with no module
// declarations and no file is still a valid installation.
func TestAnEmptyRegistryWithNoFileIsOff(t *testing.T) {
	p := registryPlugin(t)
	p.Mount(nil, wiring(nil, true))

	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
	if len(p.Menus()) != 0 {
		t.Fatalf("menus = %v, want none", p.Menus())
	}
}

func contains(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}

// TestAnOwnedEntityIsRefusedWithoutTheComposedPlugins pins fail-closed boot: an
// entity whose owner cannot be resolved has no switch, and a host that cannot
// name its plugins cannot resolve one.
func TestAnOwnedEntityIsRefusedWithoutTheComposedPlugins(t *testing.T) {
	p := registryPlugin(t, codeSpec())
	p.Mount(nil, &plugin.Context{DB: openBookkeepingDB(t)})

	err := p.Validate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "cannot list the composed plugins") {
		t.Fatalf("Validate = %v, want the composition refused", err)
	}
}

// TestAnEntityWhoseOwnerIsNotComposedIsRefused pins the other half: a
// declaration naming a plugin this build does not mount would exist with a
// switch nothing can flip, so it is refused at boot instead.
func TestAnEntityWhoseOwnerIsNotComposedIsRefused(t *testing.T) {
	p := registryPlugin(t, codeSpec())
	p.Mount(nil, &plugin.Context{
		DB:            openBookkeepingDB(t),
		ActivePlugins: func() []plugin.Plugin { return nil },
		FeatureActive: func(uint, string) bool { return true },
	})

	err := p.Validate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), `plugin "notes", which is not in this build`) {
		t.Fatalf("Validate = %v, want the missing plugin named", err)
	}
}

// TestAnOperatorDeclaredEntityHasNoSwitch pins that the declaration file keeps
// its meaning: an operator who declares an entity gets it in every workspace,
// and no plugin toggle can hide it.
func TestAnOperatorDeclaredEntityHasNoSwitch(t *testing.T) {
	p := registryPlugin(t)
	t.Setenv(EntitiesEnv, ticketDoc)

	// Every feature is off; the operator's entity must not care.
	p.Mount(nil, &plugin.Context{
		DB:            openBookkeepingDB(t),
		ActivePlugins: func() []plugin.Plugin { return []plugin.Plugin{notesOwner{}} },
		FeatureActive: func(uint, string) bool { return false },
	})
	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
	if len(p.entities) != 1 || p.entities[0].owned() {
		t.Fatalf("entities = %v, want one unowned entity", p.entities)
	}
	if p.entities[0].featureOff(1) {
		t.Fatal("an operator-declared entity reports itself as switched off")
	}
	menus := p.Menus()
	if len(menus) != 1 || menus[0].Feature != "" {
		t.Fatalf("menus = %v, want one entry with no feature", menus)
	}
}

// TestAnOwnedEntityIsRefusedWithoutTheFeatureHook pins the second half of the
// wiring: a host that can name its plugins but cannot answer whether a feature
// is on would leave the entity switched off forever.
func TestAnOwnedEntityIsRefusedWithoutTheFeatureHook(t *testing.T) {
	p := registryPlugin(t, codeSpec())
	p.Mount(nil, &plugin.Context{
		DB:            openBookkeepingDB(t),
		ActivePlugins: func() []plugin.Plugin { return []plugin.Plugin{notesOwner{}} },
	})

	err := p.Validate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "cannot answer whether a feature is on") {
		t.Fatalf("Validate = %v, want the composition refused", err)
	}
}
