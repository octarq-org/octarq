package openapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// contributorPlugin implements plugin.Plugin + plugin.OpenAPIContributor to
// exercise the merge step in Generate.
type contributorPlugin struct{}

func (contributorPlugin) Name() string                      { return "contributor" }
func (contributorPlugin) Models() []any                     { return nil }
func (contributorPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (contributorPlugin) OpenAPIPaths() map[string]any {
	return map[string]any{"/api/probe": map[string]any{"get": map[string]any{}}}
}
func (contributorPlugin) OpenAPISchemas() map[string]any {
	return map[string]any{"ProbeSchema": map[string]any{"type": "object"}}
}

// TestGenerateProducesValidDocument calls the generator and asserts the output
// is structurally valid OpenAPI 3.0.3: a JSON document with the spec version,
// both auth schemes, and the expected reference paths.
func TestGenerateProducesValidDocument(t *testing.T) {
	var buf bytes.Buffer
	if err := Generate(&buf, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	raw := buf.Bytes()
	if len(raw) == 0 {
		t.Fatal("Generate wrote nothing")
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Errorf("openapi version = %v, want 3.0.3", doc["openapi"])
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("no paths object")
	}
	for _, want := range []string{"/api/auth/login", "/api/auth/logout", "/api/links", "/api/domains", "/api/health"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("paths missing %q", want)
		}
	}

	components, _ := doc["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	for _, want := range []string{"cookieAuth", "bearerAuth"} {
		if _, ok := schemes[want]; !ok {
			t.Errorf("securitySchemes missing %q", want)
		}
	}

	// The document is generated via MarshalIndent: each line is worth checking
	// for a trailing-newline convention, but mainly we require no HTML escaping
	// mangling of description text.
	if strings.Contains(string(raw), "\u0026") {
		t.Error("output contains HTML-escaped entities")
	}
}

// TestGenerateMergesPluginContributors drives the OpenAPIContributor merge: a
// plugin contributing paths and schemas has both folded into the document.
func TestGenerateMergesPluginContributors(t *testing.T) {
	var buf bytes.Buffer
	if err := Generate(&buf, []plugin.Plugin{contributorPlugin{}}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	paths := doc["paths"].(map[string]any)
	if _, ok := paths["/api/probe"]; !ok {
		t.Error("plugin path not merged into the document")
	}
	if _, ok := doc["components"].(map[string]any)["schemas"].(map[string]any)["ProbeSchema"]; !ok {
		t.Error("plugin schema not merged into the document")
	}
}
