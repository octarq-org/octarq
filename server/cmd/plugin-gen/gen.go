package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var nameRegex = regexp.MustCompile("^[a-z][a-z0-9-]*$")

// ValidateName checks if the plugin name conforms to ^[a-z][a-z0-9-]*$.
func ValidateName(name string) error {
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q: must match ^[a-z][a-z0-9-]*$", name)
	}
	return nil
}

type templateData struct {
	Name        string
	PackageName string
	Title       string
	Description string
}

const pluginGoTmpl = `// Package {{.PackageName}} implements the {{.Name}} plugin for Octarq.
package {{.PackageName}}

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// Plugin is the exported unit a host wires up with app.App.Use({{.PackageName}}.Plugin{}).
type Plugin struct{}

// Compile-time assertions that Plugin satisfies the contracts it claims.
var (
	_ plugin.Plugin    = Plugin{}
	_ plugin.Describer = Plugin{}
)

// Name is the stable identifier — matches the frontend UIPlugin's name.
func (Plugin) Name() string { return "{{.Name}}" }

// Describe provides plugin metadata.
func (Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "{{.Title}}",
		Description:      "{{.Description}}",
		EnabledByDefault: true,
	}
}

// Models returns the GORM models this plugin owns.
func (Plugin) Models() []any { return nil }

// Mount registers the plugin's HTTP routes on the shared API mux.
func (Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	mux.Handle("GET /api/{{.Name}}/ping", ctx.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "hello from {{.Name}} plugin",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})))
}
`

const goModTmpl = `module example.com/{{.Name}}

go 1.25.13

require github.com/octarq-org/octarq v0.0.0
`

const readmeTmpl = `# {{.Title}} Plugin

{{.Description}}

## Plugin Contract
- **Name**: ` + "`{{.Name}}`" + `
- **Mount**: Registers ` + "`/api/{{.Name}}/ping`" + ` endpoint guarded by workspace tenancy.

## Usage
In your Octarq app:
` + "```go" + `
import "{{.PackageName}}"

app.Use({{.PackageName}}.Plugin{})
` + "```" + `
`

// Scaffold generates a new Octarq plugin boilerplate in <root>/<name>.
func Scaffold(root, name, desc string) error {
	if err := ValidateName(name); err != nil {
		return err
	}

	if root == "" {
		root = "."
	}

	targetDir := filepath.Join(root, name)
	if _, err := os.Stat(targetDir); err == nil {
		return fmt.Errorf("target directory %q already exists", targetDir)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	packageName := strings.ReplaceAll(name, "-", "")
	title := cases.Title(language.English).String(strings.ReplaceAll(name, "-", " "))
	if desc == "" {
		desc = fmt.Sprintf("%s plugin for Octarq", title)
	}

	data := templateData{
		Name:        name,
		PackageName: packageName,
		Title:       title,
		Description: desc,
	}

	// 1. Render plugin.go
	tmpl, err := template.New("plugin.go").Parse(pluginGoTmpl)
	if err != nil {
		return fmt.Errorf("parse plugin.go template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("render plugin.go: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("format plugin.go: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "plugin.go"), formatted, 0644); err != nil {
		return fmt.Errorf("write plugin.go: %w", err)
	}

	// 2. Render go.mod
	modTmpl, err := template.New("go.mod").Parse(goModTmpl)
	if err != nil {
		return fmt.Errorf("parse go.mod template: %w", err)
	}
	buf.Reset()
	if err := modTmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("render go.mod: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "go.mod"), buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}

	// 3. Render README.md
	readmeTemplate, err := template.New("README.md").Parse(readmeTmpl)
	if err != nil {
		return fmt.Errorf("parse README.md template: %w", err)
	}
	buf.Reset()
	if err := readmeTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("render README.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "README.md"), buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write README.md: %w", err)
	}

	return nil
}
