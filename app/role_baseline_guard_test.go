package app_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBothPctxConstructorsSetRequireRole(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller path")
	}
	appGoPath := filepath.Join(filepath.Dir(currentFile), "app.go")

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, appGoPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse app.go: %v", err)
	}

	pctxCount := 0
	requireRoleCount := 0

	ast.Inspect(node, func(n ast.Node) bool {
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		sel, ok := compLit.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "plugin" || sel.Sel.Name != "Context" {
			return true
		}

		pctxCount++

		for _, elt := range compLit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyIdent, ok := kv.Key.(*ast.Ident)
			if ok && keyIdent.Name == "RequireRole" {
				requireRoleCount++
			}
		}

		return true
	})

	if pctxCount == 0 {
		t.Fatal("expected to find plugin.Context literals in app.go, found 0")
	}

	if requireRoleCount != pctxCount {
		t.Fatalf("expected all %d plugin.Context instantiations to set RequireRole, but only %d did", pctxCount, requireRoleCount)
	}
}
