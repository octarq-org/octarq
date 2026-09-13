package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHumaGuard_NoDirectAPIRegistration ensures that no plugin or API code
// bypasses Huma for /api/* routes. All /api/* registrations must go through
// huma.Register via plugin.Context.Huma.
//
// This guards the Stage 2 decision: keep stdlib + Huma as the single API
// surface so OpenAPI never drifts.
func TestHumaGuard_NoDirectAPIRegistration(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	// Directories that may contain HTTP registrations.
	dirs := []string{
		filepath.Join(root, "plugin"),
		filepath.Join(root, "plugins"),
		filepath.Join(root, "internal", "api"),
	}
	var violations []string
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			continue
		}
		if err := filepath.Walk(d, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			// Skip generated / test files.
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			content := string(data)
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				trim := strings.TrimSpace(line)
				// Ignore comments.
				if strings.HasPrefix(trim, "//") {
					continue
				}
				// Detect direct mux.Handle / HandleFunc with /api/ prefix.
				// Allow-list: huma.Register, endpoint.Engine, and gated wrappers
				// which are infrastructure, not business routes.
				if strings.Contains(line, "/api/") && (strings.Contains(line, "Handle(") || strings.Contains(line, "HandleFunc(")) {
					// Skip if line is part of huma operation definition (Path field)
					// or specPaths constant, or is inside a comment/string that is not a registration.
					// Heuristic: if the file also contains "huma.Register" nearby, it's likely legit.
					// But a direct mux.Handle("/api/...") without huma is a violation.
					// We allow server.go specPaths and internal/server dispatch.
					if strings.Contains(path, "internal/server/server.go") && strings.Contains(line, "specPaths") {
						continue
					}
					if strings.Contains(content, "huma.Register") && strings.Contains(trim, "Path:") {
						// This is a huma.Operation definition, not a direct handle.
						continue
					}
					// If the trimmed line itself contains Handle and /api/ and is not inside huma.Operation, flag.
					// Exclude HandleRoot / HandleStatic which are for "/" and plugin SPA prefixes.
					if strings.Contains(trim, "HandleRoot") || strings.Contains(trim, "HandleStatic") {
						continue
					}
					// Allow-list: infrastructure routes that intentionally bypass Huma
					// (MCP SSE/stream are stdlib handlers, /api/v1/ is a compat shim).
					if strings.Contains(path, "internal/api/api.go") && (strings.Contains(trim, "/api/mcp/") || strings.Contains(trim, "/api/v1/")) {
						continue
					}
					// Only flag if the literal "/api/" appears inside the Handle call's pattern argument.
					// Simple check: Handle(".../api/..." or HandleFunc(".../api/..."
					if strings.Contains(trim, `"/api/`) || strings.Contains(trim, "'/api/") || strings.Contains(trim, "`/api/") {
						rel, _ := filepath.Rel(root, path)
						violations = append(violations, rel+":"+itoa(i+1)+": "+trim)
					}
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("walk %s: %v", d, err)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("direct /api/* registration without Huma found (%d):\n%s\n\nAll /api/* routes must be registered via huma.Register on plugin.Context.Huma so OpenAPI stays authoritative. If this is infrastructure (e.g. specPaths), add an allow-list entry in huma_guard_test.go.", len(violations), strings.Join(violations, "\n"))
	}
}

func repoRoot() (string, error) {
	// Walk up from this file's directory to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func itoa(n int) string {
	// avoid importing strconv just for test helper
	const digits = "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	if n < 100 {
		return string(digits[n/10]) + string(digits[n%10])
	}
	return string(digits[n/100]) + string(digits[(n/10)%10]) + string(digits[n%10])
}
