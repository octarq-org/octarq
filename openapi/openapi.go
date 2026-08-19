// Package openapi renders octarq's published API specification.
//
// The document is GENERATED, not written. It is read off the live huma API
// after the composition root has mounted every handler, so it describes exactly
// the routes the binary serves — the same document a running instance answers
// with at /openapi.json.
//
// This replaced a 1,835-line hand-written Go map literal. That literal
// documented 24 of the 101 core paths and had no mechanism that could notice:
// the CI "drift" guard re-ran the literal and compared it to itself, so it was
// green while three quarters of the API — every API token endpoint, every
// webhook endpoint, most of the auth surface — was undocumented, and while the
// error shape it promised integrators (`{"error": "..."}`) matched no response
// the server actually sent.
package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/app"
	"github.com/octarq-org/octarq/plugin"
)

// Generate boots the composition root with the given plugins, captures the
// OpenAPI document huma built from their registrations, and writes it as
// indented OpenAPI 3.0.3 JSON.
//
// It really does boot: the spec is a property of the mounted handlers, and the
// only way to know what is mounted is to mount it. The boot is hermetic — a
// throwaway SQLite file in a temp directory, no listener anyone can reach (Run
// returns as soon as the passed context is done) — and every environment
// variable it sets is set only when the caller left it unset, so an operator
// generating the spec against their own configuration still can.
func Generate(w io.Writer, plugins []plugin.Plugin) error {
	doc, err := Document(plugins)
	if err != nil {
		return err
	}
	b, err := Marshal(doc, plugins)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// Document boots the app and returns the huma-generated OpenAPI document.
func Document(plugins []plugin.Plugin) (*huma.OpenAPI, error) {
	tmp, err := os.MkdirTemp("", "octarq-openapi-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	// Defaults only — never an override. Generating the spec must not depend on
	// (or disturb) a real deployment's configuration.
	setIfUnset("OCTARQ_DB_DRIVER", "sqlite")
	setIfUnset("OCTARQ_DB_DSN", filepath.Join(tmp, "openapi.db"))
	setIfUnset("OCTARQ_SECRET_KEY", "openapi-spec-generation-key-not-a-secret")
	setIfUnset("OCTARQ_ADMIN_PASSWORD", "openapi-spec-generation-password")
	// Port 0 so a generation run can never collide with a server already
	// listening on the default port.
	setIfUnset("OCTARQ_LISTEN", "127.0.0.1:0")

	a, err := app.New()
	if err != nil {
		return nil, fmt.Errorf("boot for spec generation: %w", err)
	}
	for _, p := range plugins {
		a.Use(p)
	}
	capture := &specCapture{}
	a.Use(capture)

	// Run mounts everything, then returns immediately because the context is
	// already done. Nothing is served; the mounting is the whole point.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(ctx); err != nil {
		return nil, fmt.Errorf("mount for spec generation: %w", err)
	}
	if capture.doc == nil {
		return nil, fmt.Errorf("spec generation: no OpenAPI document was captured")
	}
	return capture.doc, nil
}

// Marshal renders the document as indented OpenAPI 3.0.3 JSON, merging in the
// routes huma cannot see (see extraPaths and OpenAPIContributor below).
//
// 3.0.3 rather than the 3.1 huma emits natively: the reference explorer, the
// SDK generators and oasdiff all have deeper 3.0 support, and a spec its
// consumers half-understand is worth less than one they fully do. A running
// instance serves both (/openapi.json is 3.1, /openapi-3.0.json is this).
func Marshal(doc *huma.OpenAPI, plugins []plugin.Plugin) ([]byte, error) {
	raw, err := doc.Downgrade()
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	paths, _ := m["paths"].(map[string]any)
	if paths == nil {
		paths = map[string]any{}
		m["paths"] = paths
	}
	schemas := componentSchemas(m)

	// Routes registered straight onto the mux never reach huma, so they are the
	// one thing generation genuinely cannot see. They are described by hand
	// here and by plugins through OpenAPIContributor — and in BOTH cases only
	// filled in where huma produced nothing, so a stale hand-written entry can
	// never override the generated truth.
	for path, item := range extraPaths() {
		if _, generated := paths[path]; !generated {
			paths[path] = item
		}
	}
	for _, p := range plugins {
		contributor, ok := p.(plugin.OpenAPIContributor)
		if !ok {
			continue
		}
		for path, item := range contributor.OpenAPIPaths() {
			if _, generated := paths[path]; !generated {
				paths[path] = item
			}
		}
		for name, schema := range contributor.OpenAPISchemas() {
			if _, generated := schemas[name]; !generated {
				schemas[name] = schema
			}
		}
	}
	if len(schemas) > 0 {
		components, _ := m["components"].(map[string]any)
		if components == nil {
			components = map[string]any{}
			m["components"] = components
		}
		components["schemas"] = schemas
	}

	// Indented, with sorted keys (encoding/json sorts map keys), so the
	// committed artifact diffs line by line and the CI drift check compares
	// content rather than formatting noise.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func componentSchemas(m map[string]any) map[string]any {
	components, _ := m["components"].(map[string]any)
	if components == nil {
		return map[string]any{}
	}
	schemas, _ := components["schemas"].(map[string]any)
	if schemas == nil {
		return map[string]any{}
	}
	return schemas
}

func setIfUnset(key, value string) {
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, value)
	}
}

// specCapture is a plugin whose only job is to hold on to the huma API it is
// mounted with.
//
// It is how generation reads the finished document without the app package
// growing a spec-shaped accessor: Mount hands every plugin the shared
// huma.API, and the document is read after Run returns, by which point every
// other plugin has registered. A plugin is already the supported way to reach
// into the composition root, so the seam that lets Pro extend octarq is the
// same seam that documents it.
type specCapture struct {
	doc *huma.OpenAPI
}

func (*specCapture) Name() string  { return "openapi-spec-capture" }
func (*specCapture) Models() []any { return nil }

func (s *specCapture) Mount(_ plugin.Mux, ctx *plugin.Context) {
	if ctx != nil && ctx.Huma != nil {
		s.doc = ctx.Huma.OpenAPI()
	}
}
