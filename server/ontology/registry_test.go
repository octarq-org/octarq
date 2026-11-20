package ontology_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/ontology"
)

const owner = "notes"

func namedSpec(slug string) ontology.Spec {
	s := validSpec()
	s.Slug = slug
	return s
}

func TestRegistryHandsBackWhatWasRegistered(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(owner, namedSpec("first"), namedSpec("second")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := r.Declared()
	if len(got) != 2 || got[0].Slug != "first" || got[1].Slug != "second" {
		t.Fatalf("Declared = %v, want first and second in registration order", slugsOf(got))
	}
}

func TestRegistryStampsTheOwner(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register("tickets", namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := r.Declared(); got[0].Owner != "tickets" {
		t.Fatalf("Owner = %q, want tickets", got[0].Owner)
	}
}

func TestRegistryRefusesADeclarationWithNoOwner(t *testing.T) {
	if err := ontology.NewRegistry().Register("", namedSpec("first")); !errors.Is(err, ontology.ErrNoOwner) {
		t.Fatalf("Register with no owner = %v, want ErrNoOwner", err)
	}
}

func TestRegistryRefusesAnOwnerThatIsNotAPluginName(t *testing.T) {
	for _, bad := range []string{"Notes", "notes plugin", "-notes", "notes!"} {
		if err := ontology.NewRegistry().Register(bad, namedSpec("first")); err == nil {
			t.Fatalf("Register with owner %q = nil, want an error", bad)
		}
	}
}

func TestRegistryDoesNotMutateTheCallersSpec(t *testing.T) {
	spec := namedSpec("first")
	r := ontology.NewRegistry()
	if err := r.Register("notes", spec); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if spec.Owner != "" {
		t.Fatalf("caller's spec Owner = %q, want it untouched", spec.Owner)
	}
}

func TestRegistryFreezesOnFirstRead(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(owner, namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	r.Declared()

	if err := r.Register(owner, namedSpec("late")); !errors.Is(err, ontology.ErrFrozen) {
		t.Fatalf("Register after Declared = %v, want ErrFrozen", err)
	}
	if got := r.Declared(); len(got) != 1 || got[0].Slug != "first" {
		t.Fatalf("second Declared = %v, want the same one entity", slugsOf(got))
	}
}

func TestRegistryHandsBackACopy(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(owner, namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	first := r.Declared()
	first[0].Slug = "mutated"

	if got := r.Declared(); got[0].Slug != "first" {
		t.Fatalf("Declared after a caller mutated its copy = %q, want first", got[0].Slug)
	}
}

func TestRegistryRefusesADuplicateSlug(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(owner, namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := r.Register(owner, namedSpec("first"))
	if err == nil || !strings.Contains(err.Error(), `"first" is registered twice`) {
		t.Fatalf("duplicate Register = %v, want a twice-registered error", err)
	}

	if err := ontology.NewRegistry().Register(owner, namedSpec("dup"), namedSpec("dup")); err == nil {
		t.Fatal("Register with a duplicate in one call = nil, want an error")
	}
}

func TestRegistryValidatesAtRegistration(t *testing.T) {
	bad := validSpec()
	bad.Slug = "Not A Slug"

	err := ontology.NewRegistry().Register(owner, bad)
	if err == nil {
		t.Fatal("Register with an invalid slug = nil, want an error")
	}
	if !hasCode(err, ontology.CodeSlugInvalid) {
		t.Fatalf("Register error = %v, want the slug issue code", err)
	}
}

func TestRegistryRefusesAnEmptyCall(t *testing.T) {
	if err := ontology.NewRegistry().Register(owner); !errors.Is(err, ontology.ErrNoSpecs) {
		t.Fatalf("Register() = %v, want ErrNoSpecs", err)
	}
}

func TestMustRegisterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister with an invalid spec did not panic")
		}
	}()
	bad := validSpec()
	bad.Slug = ""
	ontology.NewRegistry().MustRegister(owner, bad)
}

func TestPackageMustRegisterReachesTheDefaultRegistry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("ontology.MustRegister with an invalid spec did not panic")
		}
	}()
	bad := validSpec()
	bad.Slug = "Not A Slug"
	ontology.MustRegister(owner, bad)
}

func TestRegistriesAreIndependent(t *testing.T) {
	a, b := ontology.NewRegistry(), ontology.NewRegistry()
	if err := a.Register(owner, namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := b.Declared(); len(got) != 0 {
		t.Fatalf("a second registry sees %v, want nothing", slugsOf(got))
	}
}

func TestDefaultIsStable(t *testing.T) {
	first, second := ontology.Default(), ontology.Default()
	if first != second {
		t.Fatal("Default returned two registries")
	}
}

func slugsOf(specs []ontology.Spec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Slug
	}
	return out
}
