package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// findServerDir locates the server module root from this package's directory.
func findServerDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", ".."),
		filepath.Join("server"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "plugin")); err == nil {
			return c
		}
	}
	t.Fatalf("could not locate the server module root from %v", candidates)
	return ""
}

// coreGoFiles walks the server module and yields every non-test Go file.
func coreGoFiles(t *testing.T, server string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(server, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			// Vendored and generated trees are not hand-written core code.
			if name == "testdata" || name == "node_modules" || name == "webembed" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", server, err)
	}
	if len(files) == 0 {
		t.Fatalf("no Go sources found under %s", server)
	}
	return files
}

// TestWorkforceTablesStayOutOfCore guards the split that moved the workforce
// feature — agents, tasks, approvals, the spend ledger — into the Pro module.
// Core keeps only the contract in server/plugin/workforce.go.
//
// The guard is about the two ways the feature creeps back:
//
//   - a gorm model returns a "workforce_*" table name, so core migrations start
//     creating tables the Pro plugin also owns;
//   - a handler mounts a "/api/workforce..." route, so core serves the feature
//     again and the plugin's routes collide with it.
//
// Both would compile, both would pass every other test, and the second one
// shows up as a 409 in a workspace that has the plugin installed.
func TestWorkforceTablesStayOutOfCore(t *testing.T) {
	server := findServerDir(t)
	fset := token.NewFileSet()
	sawSPI := false

	for _, path := range coreGoFiles(t, server) {
		rel, err := filepath.Rel(server, path)
		if err != nil {
			rel = path
		}
		if filepath.ToSlash(rel) == "plugin/workforce.go" {
			sawSPI = true
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}

		// 1. No workforce table names outside the contract file.
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "TableName" || fn.Body == nil {
				return true
			}
			ast.Inspect(fn.Body, func(bn ast.Node) bool {
				lit, ok := bn.(*ast.BasicLit)
				if !ok {
					return true
				}
				name, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if strings.HasPrefix(name, "workforce_") {
					t.Errorf("%s: TableName() returns %q — workforce tables belong to the Pro module, core keeps only the contract", rel, name)
				}
				return true
			})
			return true
		})

		// 2. No workforce routes.
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if strings.HasPrefix(value, "/api/workforce") {
				t.Errorf("%s: mounts route %q — workforce routes belong to the Pro module", rel, value)
			}
			return true
		})
	}

	if !sawSPI {
		t.Fatalf("plugin/workforce.go is missing: core must keep the workforce contract even though it ships no implementation")
	}
}

// TestNoWorkforcePackageInCore fails when the removed implementation package
// reappears next to the other core plugins. The contract lives in plugin/, the
// implementation lives in octarq-pro.
func TestNoWorkforcePackageInCore(t *testing.T) {
	server := findServerDir(t)
	for _, dir := range []string{
		filepath.Join(server, "plugins", "workforce"),
		filepath.Join(server, "internal", "workforce"),
	} {
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("%s exists: the workforce implementation belongs to octarq-pro, not core", dir)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", dir, err)
		}
	}
}
