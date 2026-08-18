package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDefaults(t *testing.T) {
	opts, td, err := resolve(Options{Name: "my-plugin"})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if opts.Dir != "octarq-plugin-my-plugin" {
		t.Errorf("Dir = %q, want octarq-plugin-my-plugin", opts.Dir)
	}
	if td.RoutePrefix != "/api/my-plugin" {
		t.Errorf("RoutePrefix = %q, want /api/my-plugin", td.RoutePrefix)
	}
	if td.GoPackage != "myplugin" {
		t.Errorf("GoPackage = %q, want myplugin", td.GoPackage)
	}
}

func TestGenerate_CustomVersionAndTrimSpace(t *testing.T) {
	dir := t.TempDir()
	created, err := Generate(Options{
		Name:    "  custom-plugin  ",
		Dir:     dir,
		Version: "v1.2.3",
		Module:  "example.com/custom-plugin",
		NpmName: "@org/custom-plugin",
		Force:   true,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(created) == 0 {
		t.Fatal("expected created files")
	}

	modBytes, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(modBytes), "v1.2.3") {
		t.Errorf("expected custom version v1.2.3 in go.mod, got:\n%s", modBytes)
	}
}

func TestGenerate_MkdirAllError(t *testing.T) {
	tmp := t.TempDir()
	// Create a regular file where a directory is expected
	blockingFile := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blockingFile, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Try generating inside the blocking file (as if it were a directory)
	targetDir := filepath.Join(blockingFile, "invalid-dir")
	_, err := Generate(Options{
		Name: "test-err",
		Dir:  targetDir,
	})
	if err == nil {
		t.Error("expected error when target directory cannot be created, got nil")
	}
}
