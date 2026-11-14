package ontology_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/ontology"
)

// owner is the declaring plugin's Name() in these tests. The registry refuses a
// declaration without one, so every call below names it.
const owner = "notes"

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
	if err := r.Register(owner, namedSpec("first"), namedSpec("second")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := r.Declared()
	if len(got) != 2 || got[0].Slug != "first" || got[1].Slug != "second" {
		t.Fatalf("Declared = %v, want first and second in registration order", slugsOf(got))
	}
}

// TestRegistryStampsTheOwner pins that the declaring plugin's name reaches the
// spec. Boot reads it from there to decide which workspace switch governs the
// entity, so a spec that lost it would be ungated.
func TestRegistryStampsTheOwner(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register("tickets", namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := r.Declared(); got[0].Owner != "tickets" {
		t.Fatalf("Owner = %q, want tickets", got[0].Owner)
	}
}

// TestRegistryRefusesADeclarationWithNoOwner pins the fail-closed rule: an entity
// nothing can turn off is not a state a module may reach by forgetting an
// argument.
func TestRegistryRefusesADeclarationWithNoOwner(t *testing.T) {
	if err := ontology.NewRegistry().Register("", namedSpec("first")); !errors.Is(err, ontology.ErrNoOwner) {
		t.Fatalf("Register with no owner = %v, want ErrNoOwner", err)
	}
}

// TestRegistryRefusesAnOwnerThatIsNotAPluginName pins that a typo is refused at
// init. A name that cannot match a composed plugin would fail at boot instead,
// with a message about composition rather than about the argument.
func TestRegistryRefusesAnOwnerThatIsNotAPluginName(t *testing.T) {
	for _, bad := range []string{"Notes", "notes plugin", "-notes", "notes!"} {
		if err := ontology.NewRegistry().Register(bad, namedSpec("first")); err == nil {
			t.Fatalf("Register with owner %q = nil, want an error", bad)
		}
	}
}

// TestRegistryDoesNotMutateTheCallersSpec pins that the owner is stamped into the
// registry's copy. A caller that reuses its spec for a second registration under
// another owner must not carry the first one's name into it.
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

// TestRegistryFreezesOnFirstRead pins the property that makes "register at init,
// read at boot" safe: a registration that arrives after boot would reach no
// table, no route and no tool, so it is refused instead of quietly ignored.
func TestRegistryFreezesOnFirstRead(t *testing.T) {
	r := ontology.NewRegistry()
	if err := r.Register(owner, namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	r.Declared()

	if err := r.Register(owner, namedSpec("late")); !errors.Is(err, ontology.ErrFrozen) {
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
	if err := r.Register(owner, namedSpec("first")); err != nil {
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
	if err := r.Register(owner, namedSpec("first")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := r.Register(owner, namedSpec("first"))
	if err == nil || !strings.Contains(err.Error(), `"first" is registered twice`) {
		t.Fatalf("duplicate Register = %v, want a twice-registered error", err)
	}

	// Within one call as well.
	if err := ontology.NewRegistry().Register(owner, namedSpec("dup"), namedSpec("dup")); err == nil {
		t.Fatal("Register with a duplicate in one call = nil, want an error")
	}
}

// TestRegistryValidatesAtRegistration pins that a module author learns about a
// bad declaration at init rather than at boot.
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

// TestRegistryRefusesAnEmptyCall pins that a module which meant to declare
// something but passed nothing is told so.
func TestRegistryRefusesAnEmptyCall(t *testing.T) {
	if err := ontology.NewRegistry().Register(owner); !errors.Is(err, ontology.ErrNoSpecs) {
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
	ontology.NewRegistry().MustRegister(owner, bad)
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
	ontology.MustRegister(owner, bad)
}

// TestRegistriesAreIndependent pins that a test or a composition root can keep
// its declarations away from the process-wide set.
func TestRegistriesAreIndependent(t *testing.T) {
	a, b := ontology.NewRegistry(), ontology.NewRegistry()
	if err := a.Register(owner, namedSpec("first")); err != nil {
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
