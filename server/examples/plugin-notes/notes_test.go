package notes

import (
	"testing"

	"github.com/octarq-org/octarq/server/ontology"
)

func TestDeclarationIsValid(t *testing.T) {
	if err := ontology.Validate(entity); err != nil {
		t.Fatalf("declaration: %v", err)
	}
}

func TestInitRegistersTheDeclaration(t *testing.T) {
	for _, s := range ontology.Default().Declared() {
		if s.Slug == entity.Slug {
			return
		}
	}
	t.Fatalf("init did not register %q", entity.Slug)
}

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
