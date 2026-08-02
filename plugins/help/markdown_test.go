package help

import (
	"strings"
	"testing"
)

// The corpus is written for Astro Starlight, whose asides are :::type blocks.
// Before this conversion the viewer had no idea what they were, so goldmark
// passed them through as literal ":::tip" paragraphs — the reader saw the markup
// instead of the callout.
func TestRenderMarkdownConvertsAsides(t *testing.T) {
	html, err := renderMarkdown(":::tip\nSet up Short Links first.\n:::\n")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, ":::") {
		t.Errorf("aside markup leaked into the output: %s", html)
	}
	// The viewer styles callouts off the GFM tag, so that is what has to survive.
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
		// GFM has no alert for an unknown type; a note is the harmless landing
		// spot, and beats rendering the raw ":::whatever" line.
		"whatever": "NOTE",
	}
	for kind, want := range cases {
		got := convertAsides(":::" + kind + "\nbody\n:::\n")
		if !strings.Contains(got, "> [!"+want+"]") {
			t.Errorf(":::%s converted to %q, want a [!%s] alert", kind, got, want)
		}
	}
}

// A multi-paragraph aside must stay one callout: a bare blank line would end the
// blockquote and split it into two, the second one unlabelled.
func TestConvertAsidesKeepsBlankLinesInside(t *testing.T) {
	got := convertAsides(":::caution\nfirst\n\nsecond\n:::\n")
	if strings.Contains(got, "\n\nsecond") {
		t.Errorf("blank line broke the callout in two:\n%s", got)
	}
	if !strings.Contains(got, "> second") {
		t.Errorf("second paragraph left outside the callout:\n%s", got)
	}
}

// A shell snippet may legitimately contain a ":::" line, and must reach the
// reader verbatim rather than being eaten as a directive.
func TestConvertAsidesIgnoresFencedCode(t *testing.T) {
	src := "```sh\n:::tip\necho hi\n```\n"
	if got := convertAsides(src); got != src {
		t.Errorf("fenced code was rewritten:\n%s", got)
	}
}

// Starlight allows a custom title, which GFM cannot express. Dropping it would
// lose the one line that says what the callout is about.
func TestConvertAsidesKeepsCustomTitle(t *testing.T) {
	got := convertAsides(":::tip[Next Steps]\nbody\n:::\n")
	if !strings.Contains(got, "> [!TIP]") || !strings.Contains(got, "**Next Steps**") {
		t.Errorf("custom title lost:\n%s", got)
	}
}
