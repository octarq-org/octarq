package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// inTempDir runs fn with the working directory pointing at a fresh temp dir,
// so the generator's relative file writes never touch the repo.
func inTempDir(t *testing.T, fn func(dir string)) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(old)
	fn(dir)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestRunManifestErrors covers the two configuration refusals: an empty
// manifest and a manifest that is not valid JSON.
func TestRunManifestErrors(t *testing.T) {
	inTempDir(t, func(dir string) {
		t.Setenv("OCTARQ_PLUGINS", "   ")
		r := run()
		if r == nil || !strings.Contains(r.Error(), "OCTARQ_PLUGINS is empty") {
			t.Fatalf("empty manifest error = %v", r)
		}
	})
	inTempDir(t, func(dir string) {
		t.Setenv("OCTARQ_PLUGINS", "{not json")
		r := run()
		if r == nil || !strings.Contains(r.Error(), "parsing OCTARQ_PLUGINS") {
			t.Fatalf("bad json error = %v", r)
		}
	})
}

// TestRunFrontendOnlyManifest drives the npm-only path (no `go get`, so no
// network): the frontend manifest is written with the npm specifiers and the
// backend file regenerates to an empty composition.
func TestRunFrontendOnlyManifest(t *testing.T) {
	inTempDir(t, func(dir string) {
		t.Setenv("OCTARQ_PLUGINS", `[{"npm":"@acme/octarq-plugin-hello"},{"npm":"@acme/octarq-plugin-stats"}]`)
		if err := run(); err != nil {
			t.Fatalf("run: %v", err)
		}

		fe := readFile(t, dir+"/"+frontendFile)
		if !strings.Contains(fe, "@acme/octarq-plugin-hello") || !strings.Contains(fe, "@acme/octarq-plugin-stats") {
			t.Errorf("frontend manifest = %q, want both npm specifiers", fe)
		}

		gen := readFile(t, dir+"/"+genFile)
		if strings.Contains(gen, "p0") || strings.Contains(gen, `p1 "`) {
			t.Errorf("backend file with no go entries should have no plugin imports:\n%s", gen)
		}
		if !strings.Contains(gen, "return nil") {
			t.Errorf("empty backend should return nil:\n%s", gen)
		}
	})
}

// TestRunBackendManifestFailsGoGet drives the backend `go get` loop in a
// module-less temp dir (as CI does when invoked outside a checkout), where the
// resolution step fails immediately — offline and deterministically — and run
// must surface that failure.
func TestRunBackendManifestFailsGoGet(t *testing.T) {
	inTempDir(t, func(dir string) {
		t.Setenv("OCTARQ_PLUGINS", `[{"go":"example.invalid/octarq-plugin-x","npm":"@x/y"}]`)
		err := run()
		if err == nil {
			t.Fatal("run succeeded despite an unresolvable backend module")
		}
		if !strings.Contains(err.Error(), "go get") {
			t.Errorf("error = %v, want it to wrap the go get failure", err)
		}
	})
}

// TestRunEmptyManifest writes a valid empty frontend manifest and an empty
// backend composition for an explicit empty manifest.
func TestRunEmptyManifest(t *testing.T) {
	inTempDir(t, func(dir string) {
		t.Setenv("OCTARQ_PLUGINS", "[]")
		if err := run(); err != nil {
			t.Fatalf("run: %v", err)
		}
		if fe := readFile(t, dir+"/"+frontendFile); strings.TrimSpace(fe) != "[]" {
			t.Errorf("frontend manifest = %q, want []", fe)
		}
		if gen := readFile(t, dir+"/"+genFile); !strings.Contains(gen, "return nil") {
			t.Errorf("backend file should be an empty composition:\n%s", gen)
		}
	})
}

// TestRunWriteFailures covers the os.WriteFile error branches by putting
// directories in the way of the generated files.
func TestRunWriteFailures(t *testing.T) {
	inTempDir(t, func(dir string) {
		// writeBackend fails -> run surfaces it.
		if err := os.Mkdir(genFile, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", genFile, err)
		}
		t.Setenv("OCTARQ_PLUGINS", `[{"npm":"@x/y"}]`)
		if err := run(); err == nil {
			t.Fatal("run succeeded despite an unwritable backend file")
		}

		// Frontend write fails on an empty manifest.
		os.Remove(genFile)
		if err := os.Mkdir(frontendFile, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", frontendFile, err)
		}
		t.Setenv("OCTARQ_PLUGINS", "[]")
		if err := run(); err == nil {
			t.Fatal("run succeeded despite an unwritable frontend manifest")
		}
	})
}

// TestWriteBackendEntries regenerates the Go plugin set for a backend manifest,
// producing per-plugin aliases and &Plugin{} constructor lines.
func TestWriteBackendEntries(t *testing.T) {
	have := writeBackendEntries(t, []entry{
		{Go: "github.com/octarq-org/octarq/examples/plugin-hello"},
		{Go: "github.com/example/foo", GoMod: "github.com/example/foo@v1.0.0", NPM: "@example/foo"},
	})
	if !strings.Contains(have, `p0 "github.com/octarq-org/octarq/examples/plugin-hello"`) {
		t.Errorf("missing p0 aliased import:\n%s", have)
	}
	if !strings.Contains(have, `&p1.Plugin{}`) {
		t.Errorf("missing second plugin constructor:\n%s", have)
	}
}

// writeBackendEntries is a thin test driver for writeBackend that reads the
// generated file back out of the temp dir.
func writeBackendEntries(t *testing.T, entries []entry) string {
	t.Helper()
	var got string
	inTempDir(t, func(dir string) {
		if err := writeBackend(entries); err != nil {
			t.Fatalf("writeBackend: %v", err)
		}
		got = readFile(t, filepath.Join(dir, genFile))
	})
	return got
}
