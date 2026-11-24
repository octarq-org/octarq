package help

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var asideTag = map[string]string{
	"note":      "NOTE",
	"tip":       "TIP",
	"caution":   "WARNING",
	"warning":   "WARNING",
	"danger":    "CAUTION",
	"important": "IMPORTANT",
}

func convertAsides(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))

	fence := ""
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
	} else if i := strings.IndexAny(rest, " \t"); i >= 0 {
		title = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	tag, known := asideTag[strings.ToLower(strings.TrimSpace(rest))]
	if !known {
		tag = "NOTE"
	}
	return tag, title, true
}

var helpMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		renderer.WithNodeRenderers(util.Prioritized(&helpCodeRenderer{}, 100)),
	),
)

func renderMarkdown(src string) (string, error) {
	var buf bytes.Buffer
	if err := helpMarkdown.Convert([]byte(convertAsides(src)), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func parseMarkdown(src string) (ast.Node, []byte) {
	source := []byte(convertAsides(src))
	doc := helpMarkdown.Parser().Parse(text.NewReader(source))
	return doc, source
}
