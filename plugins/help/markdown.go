package help

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// asideTag maps a Starlight aside type to the GFM alert the viewer knows how to
// style. The help corpus is authored once and read twice — by the docs site
// (Astro Starlight, whose asides are `:::tip … :::`) and by this in-app viewer —
// so the syntax the writers already use has to render here rather than leak as
// literal ":::tip" text, which is what it did.
//
// GFM has no "danger", so it folds into CAUTION, the strongest alert it does
// have. Anything unrecognised renders as a plain NOTE rather than disappearing.
var asideTag = map[string]string{
	"note":      "NOTE",
	"tip":       "TIP",
	"caution":   "WARNING",
	"warning":   "WARNING",
	"danger":    "CAUTION",
	"important": "IMPORTANT",
}

// convertAsides rewrites Starlight-style container directives into GFM alert
// blockquotes:
//
//	:::tip           becomes    > [!TIP]
//	body                        > body
//	:::
//
// A custom title (`:::tip[Next steps]`) has no GFM equivalent, so it survives as
// a bold first line inside the callout instead of being dropped.
//
// Fenced code is skipped: a shell snippet is allowed to contain a ":::" line and
// must reach the reader verbatim.
func convertAsides(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))

	fence := "" // non-empty while inside a ``` / ~~~ block
	inAside := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			out = append(out, line)
			continue
		}
		if !inAside && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			fence = trimmed[:3]
			out = append(out, line)
			continue
		}

		if inAside {
			if trimmed == ":::" {
				inAside = false
				out = append(out, "")
				continue
			}
			// A blank line inside a blockquote ends it, so it has to carry the
			// marker too or the aside splits into two callouts.
			if trimmed == "" {
				out = append(out, ">")
			} else {
				out = append(out, "> "+line)
			}
			continue
		}

		if kind, title, ok := parseAsideOpener(trimmed); ok {
			inAside = true
			out = append(out, "", "> [!"+kind+"]")
			if title != "" {
				out = append(out, "> **"+title+"**", ">")
			}
			continue
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

// parseAsideOpener recognises ":::type" and ":::type[Title]" and returns the GFM
// alert tag to open with. A bare ":::" is a closer, never an opener.
func parseAsideOpener(trimmed string) (kind, title string, ok bool) {
	if !strings.HasPrefix(trimmed, ":::") {
		return "", "", false
	}
	rest := strings.TrimSpace(trimmed[3:])
	if rest == "" {
		return "", "", false
	}
	if i := strings.Index(rest, "["); i >= 0 && strings.HasSuffix(rest, "]") {
		title = rest[i+1 : len(rest)-1]
		rest = rest[:i]
	}
	tag, known := asideTag[strings.ToLower(strings.TrimSpace(rest))]
	if !known {
		tag = "NOTE"
	}
	return tag, title, true
}

// renderMarkdown turns a help page's markdown into the HTML the viewer renders.
func renderMarkdown(src string) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(convertAsides(src)), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
