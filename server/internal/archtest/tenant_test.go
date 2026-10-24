package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findTenantsqlDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "tenantsql"),
		filepath.Join("internal", "tenantsql"),
		filepath.Join("server", "internal", "tenantsql"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	t.Fatalf("could not locate internal/tenantsql directory from current working dir: %v", candidates)
	return ""
}

// TestTenantDB_NoUnscopedEscapeHatches inspects the internal/tenantsql AST at test time
// to guarantee that no unscoped escape hatch methods (such as UnscopedDangerous, Unscoped,
// RawDB, or returning raw *gorm.DB handles) exist on TenantDB.
func TestTenantDB_NoUnscopedEscapeHatches(t *testing.T) {
	dir := findTenantsqlDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	foundTenantDB := false
	parsedFiles := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		filePath := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse file %s: %v", filePath, err)
		}
		parsedFiles++
		relFile := filepath.Base(filePath)

		// 1. Check TenantDB struct definition: ensure underlying db field is unexported
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "TenantDB" {
				return true
			}
			foundTenantDB = true
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				for _, fieldName := range field.Names {
					if fieldName.IsExported() && (fieldName.Name == "DB" || strings.Contains(strings.ToLower(fieldName.Name), "db")) {
						t.Errorf("%s: TenantDB exposes exported database handle %s, violating encapsulation", relFile, fieldName.Name)
					}
				}
			}
			return true
		})

		// 2. Check all methods declared on TenantDB
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				return true
			}

			// Check receiver type
			recvType := fn.Recv.List[0].Type
			if star, ok := recvType.(*ast.StarExpr); ok {
				recvType = star.X
			}
			ident, ok := recvType.(*ast.Ident)
			if !ok || ident.Name != "TenantDB" {
				return true
			}

			methodName := fn.Name.Name

			// Forbidden method names
			lowerMethod := strings.ToLower(methodName)
			if strings.Contains(lowerMethod, "unscoped") || methodName == "RawDB" || methodName == "DB" || methodName == "GetDB" {
				t.Errorf("%s: forbidden unscoped/escape hatch method %s() defined on TenantDB", relFile, methodName)
			}

			// Inspect function body to ensure it does not directly return the raw unscoped handle (e.g. return t.db)
			var recvVarName string
			if len(fn.Recv.List[0].Names) > 0 {
				recvVarName = fn.Recv.List[0].Names[0].Name
			}

			if fn.Body != nil {
				ast.Inspect(fn.Body, func(bn ast.Node) bool {
					ret, ok := bn.(*ast.ReturnStmt)
					if !ok {
						return true
					}
					for _, expr := range ret.Results {
						// Check if expr is directly `t.db`
						if sel, ok := expr.(*ast.SelectorExpr); ok {
							if sel.Sel.Name == "db" {
								if xIdent, ok := sel.X.(*ast.Ident); ok {
									if recvVarName == "" || xIdent.Name == recvVarName {
										t.Errorf("%s: method %s() returns raw unscoped handle %s.db", relFile, methodName, xIdent.Name)
									}
								}
							}
						}
					}
					return true
				})
			}

			return true
		})
	}

	if parsedFiles == 0 {
		t.Fatalf("no Go source files parsed in %s", dir)
	}
	if !foundTenantDB {
		t.Fatalf("could not locate TenantDB type definition in %s", dir)
	}
}
