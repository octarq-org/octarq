package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMailExcludedFromBinary proves the opt-in composition model excludes a
// feature from the binary with no build tags: this edition imports only dns+links,
// so the Go linker must drop github.com/octarq-org/octarq/plugins/mail entirely.
// It is the standing regression guard for Phase 4
// (docs/PLUGIN-COMPOSITION-UNIFICATION.md), run by the normal `go test ./...`.
func TestMailExcludedFromBinary(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	const pkg = "github.com/octarq-org/octarq/examples/edition-nomail"
	bin := filepath.Join(t.TempDir(), "edition-nomail")

	if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}

	out, err := exec.Command("go", "tool", "nm", bin).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool nm: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "plugins/mail") {
		t.Fatal("plugins/mail symbols present in edition-nomail binary — opt-in exclusion is not working (a dependency is pulling mail in)")
	}
}
