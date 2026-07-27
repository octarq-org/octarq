package app_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestBothPctxConstructorsSetRecordUsage(t *testing.T) {
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
	recordUsageCount := 0

	ast.Inspect(node, func(n ast.Node) bool {
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		// Look for &plugin.Context{...} or plugin.Context{...}
		isPluginContext := false
		if sel, ok := compLit.Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "plugin" && sel.Sel.Name == "Context" {
				isPluginContext = true
			}
		}

		if isPluginContext {
			pctxCount++
			for _, elt := range compLit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if keyIdent, ok := kv.Key.(*ast.Ident); ok && keyIdent.Name == "RecordUsage" {
						recordUsageCount++
					}
				}
			}
		}
		return true
	})

	if pctxCount < 2 {
		t.Fatalf("expected at least 2 plugin.Context initializations in app.go, found %d", pctxCount)
	}
	if recordUsageCount != pctxCount {
		t.Fatalf("found %d plugin.Context initializations, but only %d set RecordUsage", pctxCount, recordUsageCount)
	}
}

func TestRecordUsageLazyResolution(t *testing.T) {
	reg := plugin.NewRegistry()
	var capturedOrgID uint
	var capturedMetric string
	var capturedN int64

	recordUsageFn := func(orgID uint, metric string, n int64) {
		// Lazily resolved: mock what app.go RecordUsage closure does
		if v, ok := reg.Lookup("cloud.usage"); ok {
			if fn, ok := v.(func(orgID uint, metric string, n int64)); ok {
				fn(orgID, metric, n)
			}
		}
	}

	// 1. Call before provide -> no-op, no panic
	recordUsageFn(1, "links", 1)
	if capturedOrgID != 0 {
		t.Fatal("expected no-op when cloud.usage is not provided")
	}

	// 2. Register service after plugin mount
	reg.Provide("cloud.usage", func(orgID uint, metric string, n int64) {
		capturedOrgID = orgID
		capturedMetric = metric
		capturedN = n
	})

	// 3. Call after provide -> resolves lazily and invokes target
	recordUsageFn(42, "links", 5)
	if capturedOrgID != 42 || capturedMetric != "links" || capturedN != 5 {
		t.Fatalf("lazy resolution failed: got org=%d metric=%s n=%d", capturedOrgID, capturedMetric, capturedN)
	}
}
