package openapi_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/openapi"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/builtin"
)

// TestGenerateValidSpec boots the real composition root, generates the spec,
// and asserts the output a downstream consumer would see.
func TestGenerateValidSpec(t *testing.T) {
	var buf bytes.Buffer
	if err := openapi.Generate(&buf, builtin.Default()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	raw := buf.Bytes()

	// ---- valid JSON ----
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		preview := string(raw)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		t.Fatalf("output is not valid JSON: %v\nfirst 500 bytes:\n%s", err, preview)
	}

	// ---- paths count ≥ 100 ----
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("missing or non-object 'paths' key in spec")
	}
	if n := len(paths); n < 100 {
		t.Errorf("paths count = %d, want ≥ 100 (guard against regression to 24-path hand-written spec)", n)
	}

	// ---- representative routes exist with correct methods ----
	// One from each major surface: auth, links, mail, dns, extra paths, abuse.
	checks := []struct {
		path   string
		method string
	}{
		{"/api/auth/login", "post"},
		{"/api/links", "get"},
		{"/api/mailboxes", "get"},
		{"/api/domains", "get"},
		{"/auth/begin/{provider}", "get"},
		{"/api/mcp/sse", "get"},
		{"/abuse", "post"},
	}
	for _, c := range checks {
		item, ok := paths[c.path]
		if !ok {
			t.Errorf("expected path %s to exist in spec", c.path)
			continue
		}
		methods, ok := item.(map[string]any)
		if !ok {
			t.Errorf("path %s: value is not a map", c.path)
			continue
		}
		if _, ok := methods[c.method]; !ok {
			var have []string
			for k := range methods {
				have = append(have, k)
			}
			t.Errorf("path %s: missing method %s (have %v)", c.path, c.method, have)
		}
	}

	// ---- no log pollution in output ----
	// Regression guard for issue #1: GORM's default logger writes "record not
	// found" to stdout, making the output non-parsable JSON.
	out := string(raw)
	if strings.Contains(out, "record not found") {
		t.Error("output contains GORM log line 'record not found'; stdout is polluted")
	}
	if strings.Contains(out, "\"level\":\"INFO\"") {
		t.Error("output contains slog INFO line; structured logs leaked into stdout")
	}
}

type contributorPlugin struct {
	name string
}

func (p contributorPlugin) Name() string                      { return p.name }
func (p contributorPlugin) Models() []any                     { return nil }
func (p contributorPlugin) Mount(plugin.Mux, *plugin.Context) {}

func (p contributorPlugin) OpenAPIPaths() map[string]any {
	return map[string]any{
		"/api/contributed/endpoint": map[string]any{
			"get": map[string]any{
				"summary": "Contributed Endpoint",
			},
		},
		"/api/links": map[string]any{
			"get": map[string]any{
				"summary": "Should Not Override Generated",
			},
		},
	}
}

func (p contributorPlugin) OpenAPISchemas() map[string]any {
	return map[string]any{
		"ContributedSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
			},
		},
	}
}

func TestMarshal_WithOpenAPIContributor(t *testing.T) {
	doc, err := openapi.Document(builtin.Default())
	if err != nil {
		t.Fatalf("Document: %v", err)
	}

	plugins := []plugin.Plugin{contributorPlugin{name: "contributor-test"}}
	raw, err := openapi.Marshal(doc, plugins)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}

	paths, ok := parsed["paths"].(map[string]any)
	if !ok {
		t.Fatal("missing paths in spec")
	}

	// Contributed endpoint should be present
	if _, ok := paths["/api/contributed/endpoint"]; !ok {
		t.Error("expected /api/contributed/endpoint to be merged from contributor")
	}

	// Generated endpoint should NOT be overridden by contributor
	linksPath, ok := paths["/api/links"].(map[string]any)
	if !ok {
		t.Fatal("missing /api/links")
	}
	linksGet, ok := linksPath["get"].(map[string]any)
	if !ok {
		t.Fatal("missing GET /api/links")
	}
	if summary, _ := linksGet["summary"].(string); summary == "Should Not Override Generated" {
		t.Error("contributor overrode generated /api/links summary")
	}

	// Contributed schema should be present
	components, ok := parsed["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("missing schemas in components")
	}
	if _, ok := schemas["ContributedSchema"]; !ok {
		t.Error("expected ContributedSchema in components.schemas")
	}
}

func TestMarshal_NilPathsAndComponents(t *testing.T) {
	emptyDoc := &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info: &huma.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
	}
	raw, err := openapi.Marshal(emptyDoc, nil)
	if err != nil {
		t.Fatalf("Marshal emptyDoc: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal empty doc output: %v", err)
	}
	paths, ok := parsed["paths"].(map[string]any)
	if !ok {
		t.Fatal("expected paths map in empty doc output")
	}
	if _, ok := paths["/auth/begin/{provider}"]; !ok {
		t.Error("expected extraPaths to be merged into empty doc")
	}
}

func TestSetIfUnset_PreservesExisting(t *testing.T) {
	key := "OCTARQ_DB_DRIVER"
	orig, had := os.LookupEnv(key)
	_ = os.Setenv(key, "sqlite")
	defer func() {
		if had {
			_ = os.Setenv(key, orig)
		} else {
			_ = os.Unsetenv(key)
		}
	}()

	var buf bytes.Buffer
	if err := openapi.Generate(&buf, builtin.Default()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if val := os.Getenv(key); val != "sqlite" {
		t.Errorf("expected %s to remain sqlite, got %s", key, val)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, os.ErrClosed
}

func TestGenerate_WriterError(t *testing.T) {
	err := openapi.Generate(errWriter{}, builtin.Default())
	if err == nil {
		t.Error("expected error when writing to errWriter, got nil")
	}
}

func TestMarshal_NilDocError(t *testing.T) {
	_, err := openapi.Marshal(nil, nil)
	if err == nil {
		t.Error("expected error when marshaling nil doc, got nil")
	}
}

func TestMarshal_ComponentsWithoutSchemas(t *testing.T) {
	doc := &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info: &huma.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Components: &huma.Components{},
	}
	plugins := []plugin.Plugin{contributorPlugin{name: "contributor-test"}}
	raw, err := openapi.Marshal(doc, plugins)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	components, ok := parsed["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("missing schemas")
	}
	if _, ok := schemas["ContributedSchema"]; !ok {
		t.Error("missing ContributedSchema")
	}
}

type duplicatePlugin struct {
	name string
}

func (p duplicatePlugin) Name() string                      { return p.name }
func (p duplicatePlugin) Models() []any                     { return nil }
func (p duplicatePlugin) Mount(plugin.Mux, *plugin.Context) {}

func TestDocument_PluginCollisionError(t *testing.T) {
	// Two plugins with the same name trigger preflightNameCollisions error
	plugins := []plugin.Plugin{
		duplicatePlugin{name: "dup"},
		duplicatePlugin{name: "dup"},
	}
	_, err := openapi.Document(plugins)
	if err == nil {
		t.Error("expected error when booting with duplicate plugin names, got nil")
	}
	var buf bytes.Buffer
	if err := openapi.Generate(&buf, plugins); err == nil {
		t.Error("expected Generate error with duplicate plugin names, got nil")
	}
}
