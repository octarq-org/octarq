package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

func matcherFor(t *testing.T, ops ...huma.Operation) func(string) bool {
	t.Helper()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1.0.0"))
	for _, op := range ops {
		huma.Register(api, op, func(ctx context.Context, in *struct {
			Slug string `path:"slug"`
			ID   string `path:"id"`
		}) (*struct{}, error) {
			return &struct{}{}, nil
		})
	}
	return PublicGETMatcher(api)
}

func publicGET(id, path string) huma.Operation {
	return huma.Operation{OperationID: id, Method: http.MethodGet, Path: path, Metadata: map[string]any{"public": true}}
}

func TestPublicGETMatcherMatchesConcreteParameterizedPath(t *testing.T) {
	m := matcherFor(t, publicGET("storefront", "/api/storefront/{slug}"))

	if !m("/api/storefront/octarq-cloud") {
		t.Error("a concrete path for a parameterized public GET must match")
	}
	if !m("/api/storefront/self-hosted") {
		t.Error("any single segment must match the parameter")
	}
}

func TestPublicGETMatcherParameterIsExactlyOneSegment(t *testing.T) {
	m := matcherFor(t, publicGET("storefront", "/api/storefront/{slug}"))

	for _, path := range []string{
		"/api/storefront",
		"/api/storefront/",
		"/api/storefront/pro/secrets",
		"/api/storefronts/pro",
		"/api/storefront/pro/{slug}",
		"/internal/api/storefront/pro",
	} {
		if m(path) {
			t.Errorf("%s must not match /api/storefront/{slug}", path)
		}
	}
}

func TestPublicGETMatcherIgnoresNonPublicAndNonGET(t *testing.T) {
	m := matcherFor(t,
		huma.Operation{OperationID: "privateGet", Method: http.MethodGet, Path: "/api/products/{id}"},
		huma.Operation{OperationID: "publicPost", Method: http.MethodPost, Path: "/api/customer/{id}/login", Metadata: map[string]any{"public": true}},
	)

	if m("/api/products/7") {
		t.Error("a GET without public metadata must not qualify")
	}
	if m("/api/customer/7/login") {
		t.Error("a public POST must not qualify — CORS here covers reads only")
	}
}
