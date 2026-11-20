package brandassets

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var archPathRe = regexp.MustCompile(`(?s)ARCH_PATH\s*=\s*\n?\s*"([^"]+)"`)

var copies = []string{
	"web/public/logo-mark.svg",
	"web/public/logo-mark-mono.svg",
	"website/src/components/Header.astro",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func TestArchPathCopiesAreInSync(t *testing.T) {
	root := repoRoot(t)

	src := filepath.Join(root, "packages/plugin-sdk/src/brand/index.tsx")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read the canonical glyph: %v", err)
	}
	m := archPathRe.FindSubmatch(b)
	if m == nil {
		t.Fatalf("no ARCH_PATH constant in %s — did the glyph move? every copy below is now unguarded", src)
		return
	}
	canonical := string(m[1])

	for _, rel := range copies {
		t.Run(rel, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatalf("read copy: %v", err)
			}
			if !strings.Contains(string(b), canonical) {
				t.Errorf("%s does not carry the current arch path.\nExpected to find:\n  %s\nUpdate it, or the brand renders differently here than in the app.", rel, canonical)
			}
		})
	}
}
