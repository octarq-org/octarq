package links

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func countPureLines(content string) int {
	count := 0
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		count++
	}
	return count
}

func TestLinksFilesSizeAndNoSizeOKGuard(t *testing.T) {
	t.Parallel()

	// Read files from disk directly to avoid hardcoded/duplicated mocks.
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("failed to glob go files: %v", err)
	}

	exemptionTag := "SIZE" + "_OK"

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", file, err)
		}

		content := string(data)
		lines := strings.Split(content, "\n")

		// 1. Assert line 1 (and the whole file) does not contain size exemption tag
		if len(lines) > 0 && strings.Contains(lines[0], exemptionTag) {
			t.Errorf("file %s has %s on line 1: %s", file, exemptionTag, lines[0])
		}
		if strings.Contains(content, exemptionTag) {
			t.Errorf("file %s contains %s exemption tag", file, exemptionTag)
		}

		// 2. Assert pure lines <= 250
		pureCount := countPureLines(content)
		if pureCount > 250 {
			t.Errorf("file %s exceeds 250 pure lines: got %d", file, pureCount)
		}
		if pureCount == 0 {
			t.Errorf("file %s has 0 pure lines (unexpected empty file)", file)
		}
	}

	// Specifically verify that key expected files exist on disk
	expectedFiles := []string{
		"plugin.go",
		"stats.go",
		"lifecycle.go",
		"routes.go",
		"stat_helpers.go",
	}

	for _, expected := range expectedFiles {
		data, err := os.ReadFile(expected)
		if err != nil {
			t.Errorf("expected file %s to exist on disk: %v", expected, err)
			continue
		}
		cnt := countPureLines(string(data))
		if cnt > 250 {
			t.Errorf("expected file %s pure lines count %d > 250", expected, cnt)
		}
	}
}
