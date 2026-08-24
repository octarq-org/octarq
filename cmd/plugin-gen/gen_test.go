package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffold_CreatesFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	name := "custom-analytics"
	desc := "Custom analytics plugin"

	if err := Scaffold(tmpDir, name, desc); err != nil {
		t.Fatalf("Scaffold failed: %v", err)
	}

	pluginDir := filepath.Join(tmpDir, name)

	// Check plugin.go
	pluginGoPath := filepath.Join(pluginDir, "plugin.go")
	pluginGoBytes, err := os.ReadFile(pluginGoPath)
	if err != nil {
		t.Fatalf("failed to read plugin.go: %v", err)
	}
	pluginGoContent := string(pluginGoBytes)
	if !strings.Contains(pluginGoContent, `func (Plugin) Name() string { return "custom-analytics" }`) {
		t.Errorf("plugin.go missing expected Name() implementation: %s", pluginGoContent)
	}
	if !strings.Contains(pluginGoContent, "package customanalytics") {
		t.Errorf("plugin.go missing package customanalytics: %s", pluginGoContent)
	}
	if !strings.Contains(pluginGoContent, "_ plugin.Plugin    = Plugin{}") {
		t.Errorf("plugin.go missing Plugin contract assertion: %s", pluginGoContent)
	}

	// Check go.mod
	goModPath := filepath.Join(pluginDir, "go.mod")
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}
	goModContent := string(goModBytes)
	if !strings.Contains(goModContent, "module example.com/custom-analytics") {
		t.Errorf("go.mod missing expected module: %s", goModContent)
	}

	// Check README.md
	readmePath := filepath.Join(pluginDir, "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	readmeContent := string(readmeBytes)
	if !strings.Contains(readmeContent, "custom-analytics") {
		t.Errorf("README.md missing plugin name: %s", readmeContent)
	}
	if !strings.Contains(readmeContent, "Mount") {
		t.Errorf("README.md missing Mount description: %s", readmeContent)
	}

	// Assert duplicate scaffold fails
	if err := Scaffold(tmpDir, name, desc); err == nil {
		t.Errorf("expected error when scaffolding into existing directory")
	}
}

func TestScaffold_InvalidName(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	invalidNames := []string{
		"",
		"123plugin",
		"-plugin",
		"Plugin",
		"plugin_name",
		"plugin.name",
		"plugin name",
		"UPPER",
	}

	for _, invalid := range invalidNames {
		err := Scaffold(tmpDir, invalid, "test desc")
		if err == nil {
			t.Errorf("expected error for invalid name %q, got nil", invalid)
		}
	}
}
