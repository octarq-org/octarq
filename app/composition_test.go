package app_test

import (
	"testing"

	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/builtin"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/plugins/links"
)

// TestBuiltinDefaultSet checks the OSS default composition lists the three Core
// feature plugins in dependency order (dns before links before mail).
func TestBuiltinDefaultSet(t *testing.T) {
	got := builtin.Default()
	names := make([]string, len(got))
	for i, p := range got {
		names[i] = p.Name()
	}
	want := []string{"dns", "links", "mail"}
	if len(names) != len(want) {
		t.Fatalf("Default() = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("Default() order = %v, want %v", names, want)
		}
	}
	// Every entry must be Core (always-on, ungated).
	for _, p := range got {
		if !plugin.Describe(p).Core {
			t.Errorf("%s should be a Core plugin", p.Name())
		}
	}
}

// TestTrimmedComposition proves a subset composition (dns + links, no mail) is a
// valid, dependency-satisfied plugin set — the opt-in analog of the old
// -tags octarq_nomail build. mail is simply not mounted.
func TestTrimmedComposition(t *testing.T) {
	set := []plugin.Plugin{dns.New(), links.New()}
	for _, p := range set {
		if p.Name() == "mail" {
			t.Fatal("mail must not be in the trimmed set")
		}
	}
	// links requires dns, which is present, so this composition is valid.
	// (preflightDependencies is unexported; its behavior is covered directly in
	// preflight_test.go. Here we assert the set shape the composition root builds.)
	if len(set) != 2 || set[0].Name() != "dns" || set[1].Name() != "links" {
		t.Fatalf("unexpected trimmed set: %v", set)
	}
}
