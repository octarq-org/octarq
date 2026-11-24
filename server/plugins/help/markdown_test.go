package help

import (
	"strings"
	"sync"
	"testing"
)

func TestRenderMarkdownConvertsAsides(t *testing.T) {
	html, err := renderMarkdown(":::tip\nSet up Short Links first.\n:::\n")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, ":::") {
		t.Errorf("aside markup leaked into the output: %s", html)
	}
	if !strings.Contains(html, "[!TIP]") {
		t.Errorf("expected a GFM TIP alert, got: %s", html)
	}
	if !strings.Contains(html, "Set up Short Links first.") {
		t.Errorf("aside body was dropped: %s", html)
	}
	if !strings.Contains(html, "<blockquote>") {
		t.Errorf("expected the aside to render as a blockquote, got: %s", html)
	}
}

func TestConvertAsidesTagMapping(t *testing.T) {
	cases := map[string]string{
		"note":      "NOTE",
		"tip":       "TIP",
		"caution":   "WARNING",
		"danger":    "CAUTION",
		"important": "IMPORTANT",
		"whatever":  "NOTE",
	}
	for kind, want := range cases {
		got := convertAsides(":::" + kind + "\nbody\n:::\n")
		if !strings.Contains(got, "> [!"+want+"]") {
			t.Errorf(":::%s converted to %q, want a [!%s] alert", kind, got, want)
		}
	}
}

func TestConvertAsidesKeepsBlankLinesInside(t *testing.T) {
	got := convertAsides(":::caution\nfirst\n\nsecond\n:::\n")
	if strings.Contains(got, "\n\nsecond") {
		t.Errorf("blank line broke the callout in two:\n%s", got)
	}
	if !strings.Contains(got, "> second") {
		t.Errorf("second paragraph left outside the callout:\n%s", got)
	}
}

func TestConvertAsidesIgnoresFencedCode(t *testing.T) {
	src := "```sh\n:::tip\necho hi\n```\n"
	if got := convertAsides(src); got != src {
		t.Errorf("fenced code was rewritten:\n%s", got)
	}
}

func TestConvertAsidesKeepsCustomTitle(t *testing.T) {
	got := convertAsides(":::tip[Next Steps]\nbody\n:::\n")
	if !strings.Contains(got, "> [!TIP]") || !strings.Contains(got, "**Next Steps**") {
		t.Errorf("custom title lost:\n%s", got)
	}
}

func TestConvertAsidesKeepsSpaceSeparatedTitle(t *testing.T) {
	got := convertAsides(":::note 架构说明\nbody\n:::\n")
	if !strings.Contains(got, "> [!NOTE]") || !strings.Contains(got, "**架构说明**") {
		t.Errorf("space-separated title lost or tag corrupted:\n%s", got)
	}

	gotCaution := convertAsides(":::caution 本功能为 Pro 专属功能\nbody\n:::\n")
	if !strings.Contains(gotCaution, "> [!WARNING]") || !strings.Contains(gotCaution, "**本功能为 Pro 专属功能**") {
		t.Errorf("space-separated caution title lost or tag corrupted:\n%s", gotCaution)
	}
}

func TestRenderMarkdownHighlightsKnownFences(t *testing.T) {
	cases := []string{
		"```go\nfmt.Println(\"hi\")\n```\n",
		"```bash\necho hi\n```\n",
		"```json\n{\"ok\": true}\n```\n",
		"```yaml\nkey: value\n```\n",
		"```javascript\nconst a = 1;\n```\n",
		"```typescript\nconst a: number = 1;\n```\n",
	}
	for _, src := range cases {
		html, err := renderMarkdown(src)
		if err != nil {
			t.Fatalf("render %q: %v", src, err)
		}
		if !strings.Contains(html, `class="chroma"`) {
			t.Errorf("expected chroma wrapper for %q, got %s", src, html)
		}
		if strings.Contains(html, `style="color`) {
			t.Errorf("expected class-based highlighting, got inline colors: %s", html)
		}
	}
}

func TestRenderMarkdownUnknownFenceFallsBackToPlain(t *testing.T) {
	html, err := renderMarkdown("```unknownlang\n<script>alert(1)</script>\n```\n")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("code was not escaped: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Errorf("expected escaped plain code, got %s", html)
	}
}

func TestRenderMarkdownConcurrentFences(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := renderMarkdown("```go\nfunc Octarq() {}\n```\n"); err != nil {
					t.Errorf("concurrent render failed: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
