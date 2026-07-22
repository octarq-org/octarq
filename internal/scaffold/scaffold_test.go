package scaffold

import (
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGenerateProducesValidSkeleton(t *testing.T) {
	dir := t.TempDir()
	created, err := Generate(Options{
		Name:    "mail-link",
		Dir:     dir,
		Module:  "github.com/acme/octarq-plugin-mail-link",
		NpmName: "@acme/octarq-plugin-mail-link",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// The Go half is named after the plugin, and the standard file set is present.
	want := []string{"go.mod", "mail-link.go", "README.md", "web/index.ts", "web/Page.tsx", "web/package.json", "web/tsconfig.json"}
	for _, f := range want {
		if !slices.Contains(created, f) {
			t.Errorf("expected %s to be generated; got %v", f, created)
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("missing generated file %s: %v", f, err)
		}
	}

	// No template placeholder may leak into any output file.
	for _, f := range created {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if strings.Contains(string(b), "{{") {
			t.Errorf("%s still contains an unrendered placeholder", f)
		}
	}

	// The generated Go must parse and be gofmt-clean.
	goPath := filepath.Join(dir, "mail-link.go")
	src, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatalf("read go file: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), goPath, src, parser.AllErrors); err != nil {
		t.Fatalf("generated Go does not parse: %v", err)
	}
	formatted, err := format.Source(src)
	if err != nil {
		t.Fatalf("gofmt: %v", err)
	}
	if string(formatted) != string(src) {
		t.Error("generated Go is not gofmt-clean")
	}

	// Substitutions landed where they matter: package name has dashes stripped,
	// the route prefix uses the raw name.
	s := string(src)
	if !strings.Contains(s, "package maillink") {
		t.Error("Go package name should have dashes stripped (maillink)")
	}
	if !strings.Contains(s, "/api/mail-link/ping") {
		t.Error("route prefix should use the raw plugin name")
	}
	// go.mod carries the requested module path.
	mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(mod), "module github.com/acme/octarq-plugin-mail-link") {
		t.Errorf("go.mod module path wrong:\n%s", mod)
	}
}

func TestGenerateDefaults(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "out")
	created, err := Generate(Options{Name: "slack", Dir: dir})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !slices.Contains(created, "slack.go") {
		t.Errorf("expected slack.go, got %v", created)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(mod), "github.com/you/octarq-plugin-slack") {
		t.Errorf("default module path missing:\n%s", mod)
	}
	if !strings.Contains(string(mod), DefaultOctarqVersion) {
		t.Errorf("default octarq version missing:\n%s", mod)
	}
	pkg, _ := os.ReadFile(filepath.Join(dir, "web", "package.json"))
	if !strings.Contains(string(pkg), `"octarq-plugin-slack"`) {
		t.Errorf("default npm name missing:\n%s", pkg)
	}
}

func TestGenerateRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "Slack", "1abc", "has space", "trailing-", "-lead", "under_score"} {
		if _, err := Generate(Options{Name: name, Dir: t.TempDir()}); err == nil {
			t.Errorf("expected error for invalid name %q", name)
		}
	}
}

func TestGenerateRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{Name: "slack", Dir: dir}); err == nil {
		t.Error("expected refusal to write into a non-empty directory")
	}
	if _, err := Generate(Options{Name: "slack", Dir: dir, Force: true}); err != nil {
		t.Errorf("Force should allow overwrite: %v", err)
	}
}
