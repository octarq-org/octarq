package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHumaGuard_NoDirectAPIRegistration(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
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
				if strings.HasPrefix(trim, "//") {
					continue
				}
				if strings.Contains(line, "/api/") && (strings.Contains(line, "Handle(") || strings.Contains(line, "HandleFunc(")) {
					if strings.Contains(path, "internal/server/server.go") && strings.Contains(line, "specPaths") {
						continue
					}
					if strings.Contains(content, "huma.Register") && strings.Contains(trim, "Path:") {
						continue
					}
					if strings.Contains(trim, "HandleRoot") || strings.Contains(trim, "HandleStatic") {
						continue
					}
					if strings.Contains(path, "internal/api/") && strings.Contains(trim, "/api/mcp/") {
						continue
					}
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
	const digits = "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	if n < 100 {
		return string(digits[n/10]) + string(digits[n%10])
	}
	return string(digits[n/100]) + string(digits[(n/10)%10]) + string(digits[n%10])
}
