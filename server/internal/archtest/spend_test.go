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

func coreGoFiles(t *testing.T, server string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(server, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
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

func TestSpendTablesStayOutOfCore(t *testing.T) {
	server := findServerDir(t)
	fset := token.NewFileSet()
	sawSPI := false

	for _, path := range coreGoFiles(t, server) {
		rel, err := filepath.Rel(server, path)
		if err != nil {
			rel = path
		}
		slashRel := filepath.ToSlash(rel)
		if slashRel == "plugin/workforce.go" {
			t.Errorf("plugin/workforce.go still exists: workforce contract was replaced by plugin/spend.go")
			continue
		}
		if slashRel == "plugin/spend.go" {
			sawSPI = true
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}

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
				if strings.HasPrefix(name, "workforce_") || strings.HasPrefix(name, "spend_") {
					t.Errorf("%s: TableName() returns %q — spend tables belong to the Pro module, core keeps only the contract", rel, name)
				}
				return true
			})
			return true
		})

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if strings.HasPrefix(value, "/api/workforce") || strings.HasPrefix(value, "/api/spend") {
				t.Errorf("%s: mounts route %q — spend routes belong to the Pro module", rel, value)
			}
			return true
		})
	}

	if !sawSPI {
		t.Fatalf("plugin/spend.go is missing: core must keep the spend contract even though it ships no implementation")
	}
}

func TestNoSpendPackageInCore(t *testing.T) {
	server := findServerDir(t)
	for _, dir := range []string{
		filepath.Join(server, "plugins", "workforce"),
		filepath.Join(server, "internal", "workforce"),
		filepath.Join(server, "plugins", "spend"),
		filepath.Join(server, "internal", "spend"),
	} {
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("%s exists: the spend implementation belongs to octarq-pro, not core", dir)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", dir, err)
		}
	}
}
