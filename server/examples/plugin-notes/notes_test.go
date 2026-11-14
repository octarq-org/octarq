package notes

import (
	"testing"

	"github.com/octarq-org/octarq/server/ontology"
)

// TestDeclarationIsValid fails here rather than at boot. A declaration is data,
// and data the platform cannot apply is a mistake the author can see in a unit
// test — the runtime answer is a process that refuses to start.
func TestDeclarationIsValid(t *testing.T) {
	if err := ontology.Validate(entity); err != nil {
		t.Fatalf("declaration: %v", err)
	}
}

// TestInitRegistersTheDeclaration pins the seam the example exists to show:
// init() put this spec in the process-wide registry, which is what the ontology
// plugin reads once at boot. Declared() freezes the registry, so this must run
// after nothing else registers — nothing in this package does.
func TestInitRegistersTheDeclaration(t *testing.T) {
	for _, s := range ontology.Default().Declared() {
		if s.Slug == entity.Slug {
			return
		}
	}
	t.Fatalf("init did not register %q", entity.Slug)
}

// TestPluginRequiresTheOntologyPlugin pins the fail-closed half: the plugin is
// a shell around a declaration, so a build without the ontology plugin must
// refuse to compose rather than mount a plugin with no surface.
func TestPluginRequiresTheOntologyPlugin(t *testing.T) {
	p := Plugin{}
	if p.Name() != "notes" {
		t.Errorf("Name = %q, want notes", p.Name())
	}
	info := p.Describe()
	if len(info.Requires) != 1 || info.Requires[0] != "ontology" {
		t.Errorf("Requires = %v, want [ontology]", info.Requires)
	}
	if models := p.Models(); models != nil {
		t.Errorf("Models = %v, want nil: the ontology plugin owns the table", models)
	}
}
