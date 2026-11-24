package api

import (
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

func TestDocumentErrorCodes(t *testing.T) {
	documentErrorCodes(nil)
	documentErrorCodes(&huma.OpenAPI{})
	documentErrorCodes(&huma.OpenAPI{Components: &huma.Components{}})

	empty := &huma.OpenAPI{Components: &huma.Components{Schemas: huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)}}
	documentErrorCodes(empty)

	doc := &huma.OpenAPI{Components: &huma.Components{Schemas: huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)}}
	doc.Components.Schemas.Map()["Error"] = &huma.Schema{Properties: map[string]*huma.Schema{"code": {}}}
	documentErrorCodes(doc)
	code := doc.Components.Schemas.SchemaFromRef("#/components/schemas/Error").Properties["code"]
	if len(code.Enum) == 0 || code.Description == "" {
		t.Errorf("error code schema not documented: %+v", code)
	}
}

func TestDeprecationOf(t *testing.T) {
	cases := []struct {
		name       string
		op         *huma.Operation
		wantSunset string
		want       bool
	}{
		{"nil operation", nil, "", false},
		{"no metadata", &huma.Operation{}, "", false},
		{"bool true", &huma.Operation{Metadata: map[string]any{"deprecated": true}}, "", true},
		{"bool false", &huma.Operation{Metadata: map[string]any{"deprecated": false}}, "", false},
		{"empty string", &huma.Operation{Metadata: map[string]any{"deprecated": ""}}, "", false},
		{"rfc3339", &huma.Operation{Metadata: map[string]any{"deprecated": "2027-01-02T03:04:05Z"}}, "2027-01-02T03:04:05Z", true},
		{"date only", &huma.Operation{Metadata: map[string]any{"deprecated": "2027-01-02"}}, "2027-01-02T00:00:00Z", true},
		{"invalid string", &huma.Operation{Metadata: map[string]any{"deprecated": "soon"}}, "", true},
		{"other type", &huma.Operation{Metadata: map[string]any{"deprecated": 1}}, "", false},
	}

	for _, tc := range cases {
		sunset, deprecated := deprecationOf(tc.op)
		if deprecated != tc.want {
			t.Errorf("%s: deprecated = %v, want %v", tc.name, deprecated, tc.want)
		}
		got := ""
		if !sunset.IsZero() {
			got = sunset.UTC().Format(time.RFC3339)
		}
		if got != tc.wantSunset {
			t.Errorf("%s: sunset = %q, want %q", tc.name, got, tc.wantSunset)
		}
	}
}

func TestApplyDeprecation(t *testing.T) {
	applyDeprecation(nil, nil)

	plain := &huma.Operation{}
	applyDeprecation(nil, plain)
	if plain.Deprecated {
		t.Error("plain operation marked deprecated")
	}

	dated := &huma.Operation{Metadata: map[string]any{"deprecated": "2027-01-02T03:04:05Z"}}
	applyDeprecation(nil, dated)
	if !dated.Deprecated {
		t.Error("dated operation not marked deprecated")
	}
	if dated.Extensions["x-sunset"] != "2027-01-02T03:04:05Z" {
		t.Errorf("x-sunset = %v", dated.Extensions["x-sunset"])
	}

	flagOnly := &huma.Operation{Metadata: map[string]any{"deprecated": true}}
	applyDeprecation(nil, flagOnly)
	if !flagOnly.Deprecated || flagOnly.Extensions != nil {
		t.Errorf("flag-only operation = %+v", flagOnly)
	}
}

func TestNormalizeValidationResponse(t *testing.T) {
	normalizeValidationResponse(nil, nil)

	noResponses := &huma.Operation{}
	normalizeValidationResponse(nil, noResponses)

	missing := &huma.Operation{Responses: map[string]*huma.Response{"200": {}}}
	normalizeValidationResponse(nil, missing)
	if _, ok := missing.Responses["422"]; ok {
		t.Error("422 added to operation without one")
	}

	promoted := &huma.Operation{Responses: map[string]*huma.Response{"422": {Description: "Unprocessable Entity"}}}
	normalizeValidationResponse(nil, promoted)
	if promoted.Responses["400"] == nil || promoted.Responses["422"] != nil {
		t.Errorf("422 not promoted to 400: %+v", promoted.Responses)
	}

	taken := &huma.Operation{Responses: map[string]*huma.Response{"400": {}, "422": {}}}
	normalizeValidationResponse(nil, taken)
	if taken.Responses["422"] != nil || taken.Responses["400"] == nil {
		t.Errorf("422 not removed when 400 exists: %+v", taken.Responses)
	}
}
