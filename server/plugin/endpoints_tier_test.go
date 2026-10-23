package plugin_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestTierGateAllows(t *testing.T) {
	cases := []struct {
		name string
		gate plugin.TierGate
		tier string
		want bool
	}{
		{"nil gate allows a paid tier", nil, "pro", true},
		{"nil gate allows free", nil, "", true},
		{"empty tier is free under a denying gate", func(string) bool { return false }, "", true},
		{"gate allows a covered tier", func(tier string) bool { return tier == "pro" }, "pro", true},
		{"gate denies an uncovered tier", func(tier string) bool { return tier == "pro" }, "elite", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.gate.Allows(tc.tier); got != tc.want {
				t.Errorf("Allows(%q) = %v, want %v", tc.tier, got, tc.want)
			}
		})
	}
}

// TestEndpointSpecRegisterHTTPCarriesTier pins the HTTP half of a tier
// declaration: the operation metadata is what the platform gate reads, so a
// tiered endpoint must publish it or the route answers 200 to an unlicensed
// caller.
func TestEndpointSpecRegisterHTTPCarriesTier(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test", "1.0"))

	spec := plugin.EndpointSpec[echoIn, echoOut]{
		Name:    "tiered",
		Method:  "POST",
		Path:    "/api/tiered",
		Tier:    "pro",
		Handler: func(_ context.Context, in echoIn) (*echoOut, error) { return &echoOut{Reply: in.Message}, nil },
	}
	if err := spec.RegisterHTTP(api, plugin.HTTPOptions{}); err != nil {
		t.Fatalf("RegisterHTTP: %v", err)
	}

	item := api.OpenAPI().Paths["/api/tiered"]
	if item == nil || item.Post == nil {
		t.Fatal("operation /api/tiered not registered")
	}
	if got, _ := item.Post.Metadata[plugin.MetadataTier].(string); got != "pro" {
		t.Errorf("operation tier metadata = %q, want %q", got, "pro")
	}
}

// TestEndpointSpecRegisterHTTPWithoutTierOmitsMetadata pins the other side: a
// free endpoint publishes no tier, so the gate passes it through.
func TestEndpointSpecRegisterHTTPWithoutTierOmitsMetadata(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test", "1.0"))

	spec := plugin.EndpointSpec[echoIn, echoOut]{
		Name:    "free",
		Method:  "POST",
		Path:    "/api/free",
		Handler: func(_ context.Context, in echoIn) (*echoOut, error) { return &echoOut{Reply: in.Message}, nil },
	}
	if err := spec.RegisterHTTP(api, plugin.HTTPOptions{}); err != nil {
		t.Fatalf("RegisterHTTP: %v", err)
	}
	item := api.OpenAPI().Paths["/api/free"]
	if item == nil || item.Post == nil {
		t.Fatal("operation /api/free not registered")
	}
	if _, ok := item.Post.Metadata[plugin.MetadataTier]; ok {
		t.Error("free endpoint must not declare a tier")
	}
}
