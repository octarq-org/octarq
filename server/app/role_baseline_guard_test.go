package app_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBothPctxConstructorsSetHost(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller path")
	}
	hostRuntimePath := filepath.Join(filepath.Dir(currentFile), "host_runtime.go")

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, hostRuntimePath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse host_runtime.go: %v", err)
	}

	pctxCount := 0
	hostCount := 0

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
			if ok {
				if keyIdent.Name == "Host" {
					hostCount++
				}
			}
		}

		return true
	})

	if pctxCount == 0 {
		t.Fatal("expected to find plugin.Context literals in host_runtime.go, found 0")
	}

	if hostCount != pctxCount {
		t.Fatalf("expected all %d plugin.Context instantiations to set Host, but only %d did", pctxCount, hostCount)
	}
}
