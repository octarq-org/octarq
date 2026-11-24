package api

import (
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

func TestDeprecationMetadataGeoSlice(t *testing.T) {
	cases := []struct {
		name       string
		op         *huma.Operation
		wantSunset string
		want       bool
	}{
		{"nil", nil, "", false},
		{"no metadata", &huma.Operation{}, "", false},
		{"bool", &huma.Operation{Metadata: map[string]any{"deprecated": true}}, "", true},
		{"empty", &huma.Operation{Metadata: map[string]any{"deprecated": ""}}, "", false},
		{"rfc3339", &huma.Operation{Metadata: map[string]any{"deprecated": "2027-01-02T03:04:05Z"}}, "2027-01-02T03:04:05Z", true},
		{"date", &huma.Operation{Metadata: map[string]any{"deprecated": "2027-01-02"}}, "2027-01-02T00:00:00Z", true},
		{"invalid", &huma.Operation{Metadata: map[string]any{"deprecated": "soon"}}, "", true},
		{"other", &huma.Operation{Metadata: map[string]any{"deprecated": 1}}, "", false},
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

func TestApplyDeprecationGeoSlice(t *testing.T) {
	applyDeprecation(nil, nil)
	op := &huma.Operation{Metadata: map[string]any{"deprecated": "2027-01-02T03:04:05Z"}}
	applyDeprecation(nil, op)
	if !op.Deprecated || op.Extensions["x-sunset"] != "2027-01-02T03:04:05Z" {
		t.Errorf("deprecation = %+v", op)
	}
}
