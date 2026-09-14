package main

import (
	"path/filepath"
	"testing"
)

func TestCLIPluginCommand(t *testing.T) {
	t.Parallel()

	if code := runPluginCommand(nil); code != 2 {
		t.Errorf("runPluginCommand(nil) expected 2, got %d", code)
	}

	if code := runPluginCommand([]string{"unknown"}); code != 2 {
		t.Errorf("runPluginCommand(unknown) expected 2, got %d", code)
	}

	// Scaffold plugin into temp dir
	targetDir := filepath.Join(t.TempDir(), "scaffold-test")
	code := runPluginCommand([]string{"new", "testplugin", "--dir", targetDir})
	if code != 0 {
		t.Fatalf("runPluginCommand(new testplugin) expected 0, got %d", code)
	}
}

func TestCustomPluginsAndRestoreUsage(t *testing.T) {
	t.Parallel()

	cp := customPlugins()
	if cp != nil {
		t.Errorf("customPlugins() expected nil, got %v", cp)
	}

	if code := runRestoreCommand(nil); code != 1 {
		t.Errorf("runRestoreCommand(nil) expected 1, got %d", code)
	}
}
