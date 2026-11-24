package help

import (
	"bytes"
	"html"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

var helpChromaFormatter = chromahtml.New(chromahtml.WithClasses(true))

type helpCodeRenderer struct{}

func (r *helpCodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
}

func (r *helpCodeRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	block := node.(*ast.FencedCodeBlock)
	writeHelpHighlightedCode(w, string(block.Language(source)), helpCodeBlockText(block, source))
	return ast.WalkSkipChildren, nil
}

func (r *helpCodeRenderer) renderCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	writeHelpHighlightedCode(w, "", helpCodeBlockText(node, source))
	return ast.WalkSkipChildren, nil
}

func helpCodeBlockText(node ast.Node, source []byte) string {
	lines := node.Lines()
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		b.Write(segment.Value(source))
	}
	return b.String()
}

func helpChromaLexer(language string) chroma.Lexer {
	name := ""
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "bash", "sh", "shell", "zsh":
		name = "bash"
	case "json":
		name = "json"
	case "yaml", "yml":
		name = "yaml"
	case "go":
		name = "go"
	case "javascript", "js", "jsx":
		name = "javascript"
	case "typescript", "ts", "tsx":
		name = "typescript"
	}
	if name != "" {
		if lexer := lexers.Get(name); lexer != nil {
			return chroma.Coalesce(lexer)
		}
	}
	return chroma.Coalesce(lexers.Fallback)
}

func writeHelpHighlightedCode(w util.BufWriter, language, code string) {
	lexer := helpChromaLexer(language)
	var buf bytes.Buffer
	iterator, err := lexer.Tokenise(nil, code)
	if err == nil {
		err = helpChromaFormatter.Format(&buf, styles.Fallback, iterator)
	}
	if err != nil {
		w.WriteString("<pre><code>")
		w.WriteString(html.EscapeString(code))
		w.WriteString("</code></pre>\n")
		return
	}
	out := buf.String()
	if language != "" {
		tag := `<code class="language-` + html.EscapeString(strings.ToLower(strings.TrimSpace(language))) + `">`
		out = strings.Replace(out, "<code>", tag, 1)
	}
	w.WriteString(out)
	w.WriteByte('\n')
}
