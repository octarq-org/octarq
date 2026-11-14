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

// registryPlugin returns a plugin reading its own registry, with no declaration
// file in the environment.
func registryPlugin(t *testing.T, specs ...core.Spec) *Plugin {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv(EntitiesEnv, "")
	t.Setenv(EntitiesFileEnv, "")

	r := core.NewRegistry()
	if len(specs) > 0 {
		if err := r.Register(specs...); err != nil {
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
	p.Mount(nil, &plugin.Context{DB: db})

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
	p.Mount(nil, &plugin.Context{DB: db})

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

	p.Mount(nil, &plugin.Context{DB: openBookkeepingDB(t)})

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
	p.Mount(nil, &plugin.Context{DB: openBookkeepingDB(t)})

	if err := p.Validate(context.Background(), nil); !errors.Is(err, ErrNoRegistry) {
		t.Fatalf("Validate = %v, want ErrNoRegistry", err)
	}
}

// TestAnEmptyRegistryWithNoFileIsOff pins that a build with no module
// declarations and no file is still a valid installation.
func TestAnEmptyRegistryWithNoFileIsOff(t *testing.T) {
	p := registryPlugin(t)
	p.Mount(nil, &plugin.Context{})

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
