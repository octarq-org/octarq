package usagemetric

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// canonicalNames is the product list of metered metric names. The tests below
// pin it against the canonical allowed set rather than asserting the
// constants against themselves: a value drift ("mailOut" back to "mail") must
// turn red, not echo.
func canonicalNames() []string {
	return []string{Clicks, MailOut, MailIn, RawBytes}
}

func canonicalSet() map[string]bool {
	set := make(map[string]bool, len(canonicalNames()))
	for _, name := range canonicalNames() {
		set[name] = true
	}
	return set
}

// TestCanonicalMetricNames pins the constants to the metric names the quota
// side (downstream quota metricNames) enforces: clicks, mailOut, mailIn,
// plus mail.raw_bytes which has no quota consumer yet. Changing a constant's
// value so it no longer matches this set is a regression.
func TestCanonicalMetricNames(t *testing.T) {
	want := map[string]bool{
		"clicks":         true,
		"mailOut":        true,
		"mailIn":         true,
		"mail.raw_bytes": true,
	}
	seen := map[string]bool{}
	for _, name := range canonicalNames() {
		seen[name] = true
		if !want[name] {
			t.Errorf("usagemetric declares %q, which is not in the canonical allowed set %v", name, want)
		}
	}
	for w := range want {
		if !seen[w] {
			t.Errorf("canonical allowed set expects %q, but no usagemetric constant declares it", w)
		}
	}
}

// TestRecordUsageCallSitesUseCanonicalMetrics scans every .go file in the
// module and fails if any recordUsage/RecordUsage call passes a string literal
// that is not one of the canonical metric names. This is the guard against the
// two-list drift this package exists to prevent: reintroducing a literal
// ("mail") at a call site compiles fine, so it must be caught here.
func TestRecordUsageCallSitesUseCanonicalMetrics(t *testing.T) {
	canon := canonicalSet()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("module root: %v", err)
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "webembed" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	fset := token.NewFileSet()
	var violations []string
	for _, file := range files {
		node, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "recordUsage" && sel.Sel.Name != "RecordUsage") {
				return true
			}
			if len(call.Args) < 2 {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if !canon[val] {
				pos := fset.Position(lit.Pos())
				violations = append(violations, fmt.Sprintf("%s: recordUsage metric %q is not in the canonical set %v", pos, val, canonicalNames()))
			}
			return true
		})
	}
	if len(violations) > 0 {
		t.Errorf("non-canonical metric literal(s):\n%s", strings.Join(violations, "\n"))
	}
}
