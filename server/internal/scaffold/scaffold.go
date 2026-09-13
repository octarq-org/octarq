// Package scaffold generates a new octarq plugin skeleton — the Go half plus the
// mirror JS half — from an embedded template tree. It backs `octarq plugin new`.
package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates
var templatesFS embed.FS

// DefaultOctarqVersion is the github.com/octarq-org/octarq version pinned in a
// generated plugin's go.mod. It tracks the latest published module tag.
const DefaultOctarqVersion = "v0.3.0"

// nameRE constrains plugin names to a lowercase, DNS-ish token so they are safe
// as a route segment, an npm package suffix, and (dashes stripped) a Go package.
// Dashes must separate alphanumeric groups — no leading, trailing, or doubled
// dashes.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// Options configures a scaffold run.
type Options struct {
	Name    string // plugin name, e.g. "slack" (validated against nameRE)
	Dir     string // output directory; defaults to "octarq-plugin-<name>"
	Module  string // Go module path; defaults to "github.com/you/octarq-plugin-<name>"
	NpmName string // npm package name; defaults to "octarq-plugin-<name>"
	Version string // octarq module version for go.mod; defaults to DefaultOctarqVersion
	Force   bool   // overwrite a non-empty output directory
}

// templateData is the value exposed to every template.
type templateData struct {
	Name          string
	GoPackage     string
	Title         string
	Module        string
	NpmName       string
	RoutePrefix   string
	OctarqVersion string
}

// goPackage turns a plugin name into a valid Go package identifier by dropping
// the dashes ("mail-link" -> "maillink").
func goPackage(name string) string { return strings.ReplaceAll(name, "-", "") }

// title turns a plugin name into a display title ("mail-link" -> "Mail Link").
func title(name string) string {
	parts := strings.Split(name, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// resolve fills in defaults and validates o, returning the template data.
func resolve(o Options) (Options, templateData, error) {
	name := strings.TrimSpace(o.Name)
	if !nameRE.MatchString(name) {
		return o, templateData{}, fmt.Errorf("invalid plugin name %q: use lowercase letters, digits and dashes, starting with a letter", o.Name)
	}
	o.Name = name
	if o.Dir == "" {
		o.Dir = "octarq-plugin-" + name
	}
	if o.Module == "" {
		o.Module = "github.com/you/octarq-plugin-" + name
	}
	if o.NpmName == "" {
		o.NpmName = "octarq-plugin-" + name
	}
	if o.Version == "" {
		o.Version = DefaultOctarqVersion
	}
	td := templateData{
		Name:          name,
		GoPackage:     goPackage(name),
		Title:         title(name),
		Module:        o.Module,
		NpmName:       o.NpmName,
		RoutePrefix:   "/api/" + name,
		OctarqVersion: o.Version,
	}
	return o, td, nil
}

// Generate writes a new plugin skeleton into o.Dir and returns the list of
// files created (relative to o.Dir). It refuses to write into a non-empty
// directory unless o.Force is set.
func Generate(o Options) ([]string, error) {
	o, td, err := resolve(o)
	if err != nil {
		return nil, err
	}

	if entries, err := os.ReadDir(o.Dir); err == nil && len(entries) > 0 && !o.Force {
		return nil, fmt.Errorf("output directory %q is not empty (use --force to overwrite)", o.Dir)
	}

	var created []string
	err = fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// path is "templates/…"; strip the root and the .tmpl suffix. The plugin
		// Go file is named after the plugin, not "plugin.go".
		rel := strings.TrimPrefix(path, "templates/")
		rel = strings.TrimSuffix(rel, ".tmpl")
		if rel == "plugin.go" {
			rel = td.Name + ".go"
		}

		raw, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		tmpl, err := template.New(rel).Parse(string(raw))
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", path, err)
		}
		var buf strings.Builder
		if err := tmpl.Execute(&buf, td); err != nil {
			return fmt.Errorf("rendering template %s: %w", path, err)
		}

		outPath := filepath.Join(o.Dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, []byte(buf.String()), 0o644); err != nil {
			return err
		}
		created = append(created, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}
