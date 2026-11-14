package ontology_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/ontology"
)

// namedSpec is validSpec under another slug, so a test can register two.
func namedSpec(slug string) ontology.Spec {
	s := validSpec()
	s.Slug = slug
	return s
}

// TestRegistryHandsBackWhatWasRegistered pins the happy path: a module declares
// at init, boot reads the same entities.
func TestRegistryHandsBackWhatWasRegistered(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(namedSpec("first"), namedSpec("second")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := r.Declared()
	if len(got) != 2 || got[0].Slug != "first" || got[1].Slug != "second" {
		t.Fatalf("Declared = %v, want first and second in registration order", slugsOf(got))
	}
}

// TestRegistryFreezesOnFirstRead pins the property that makes "register at init,
// read at boot" safe: a registration that arrives after boot would reach no
// table, no route and no tool, so it is refused instead of quietly ignored.
func TestRegistryFreezesOnFirstRead(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	r.Declared()

	if err := r.Register(namedSpec("late")); !errors.Is(err, ontology.ErrFrozen) {
		t.Fatalf("Register after Declared = %v, want ErrFrozen", err)
	}
	// The frozen set is stable, so a second reader (the MCP mount) sees the same
	// entities rather than an empty set or a panic.
	if got := r.Declared(); len(got) != 1 || got[0].Slug != "first" {
		t.Fatalf("second Declared = %v, want the same one entity", slugsOf(got))
	}
}

// TestRegistryHandsBackACopy pins that a reader cannot alter what the next reader
// sees.
func TestRegistryHandsBackACopy(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	first := r.Declared()
	first[0].Slug = "mutated"

	if got := r.Declared(); got[0].Slug != "first" {
		t.Fatalf("Declared after a caller mutated its copy = %q, want first", got[0].Slug)
	}
}

// TestRegistryRefusesADuplicateSlug pins that two declarations of one table are
// refused at the point the second is registered, not at boot with an index.
func TestRegistryRefusesADuplicateSlug(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := r.Register(namedSpec("first"))
	if err == nil || !strings.Contains(err.Error(), `"first" is registered twice`) {
		t.Fatalf("duplicate Register = %v, want a twice-registered error", err)
	}

	// Within one call as well.
	if err := ontology.NewRegistry().Register(namedSpec("dup"), namedSpec("dup")); err == nil {
		t.Fatal("Register with a duplicate in one call = nil, want an error")
	}
}

// TestRegistryValidatesAtRegistration pins that a module author learns about a
// bad declaration at init rather than at boot.
func TestRegistryValidatesAtRegistration(t *testing.T) {
	bad := validSpec()
	bad.Slug = "Not A Slug"

	err := ontology.NewRegistry().Register(bad)
	if err == nil {
		t.Fatal("Register with an invalid slug = nil, want an error")
	}
	if !hasCode(err, ontology.CodeSlugInvalid) {
		t.Fatalf("Register error = %v, want the slug issue code", err)
	}
}

// TestRegistryRefusesAnEmptyCall pins that a module which meant to declare
// something but passed nothing is told so.
func TestRegistryRefusesAnEmptyCall(t *testing.T) {
	if err := ontology.NewRegistry().Register(); !errors.Is(err, ontology.ErrNoSpecs) {
		t.Fatalf("Register() = %v, want ErrNoSpecs", err)
	}
}

// TestMustRegisterPanics pins the init-time form: there is no caller to return
// to, so a refused declaration stops the process.
func TestMustRegisterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister with an invalid spec did not panic")
		}
	}()
	bad := validSpec()
	bad.Slug = ""
	ontology.NewRegistry().MustRegister(bad)
}

// TestPackageMustRegisterReachesTheDefaultRegistry pins the init form a module
// actually calls. It registers nothing valid here: a good declaration would
// freeze the process-wide set for every later test. The Pro e2e suite covers the
// success path end to end.
func TestPackageMustRegisterReachesTheDefaultRegistry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("ontology.MustRegister with an invalid spec did not panic")
		}
	}()
	bad := validSpec()
	bad.Slug = "Not A Slug"
	ontology.MustRegister(bad)
}

// TestRegistriesAreIndependent pins that a test or a composition root can keep
// its declarations away from the process-wide set.
func TestRegistriesAreIndependent(t *testing.T) {
	a, b := ontology.NewRegistry(), ontology.NewRegistry()
	if err := a.Register(namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := b.Declared(); len(got) != 0 {
		t.Fatalf("a second registry sees %v, want nothing", slugsOf(got))
	}
}

// TestDefaultIsStable pins that Default answers the same registry every time: a
// module registers into it from init, and the plugin reads it later.
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
