package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// PublicGETMatcher is asked about the CONCRETE request path (middleware.go's
// applyCORS passes r.URL.Path), while the OpenAPI document it is built from
// holds TEMPLATES. These tests pin that translation in both directions: a
// parameterized public route must be reachable cross-origin, and matching must
// stay strictly per-segment so nothing inherits access from a neighbour.
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

// The regression this file exists for: with a plain map lookup the concrete
// path never equals the template, so every parameterized public route was
// refused cross-origin — the route is public, the browser is blocked, and the
// page renders nothing with no error anywhere.
func TestPublicGETMatcherMatchesConcreteParameterizedPath(t *testing.T) {
	m := matcherFor(t, publicGET("storefront", "/api/storefront/{slug}"))

	if !m("/api/storefront/octarq-cloud") {
		t.Error("a concrete path for a parameterized public GET must match")
	}
	if !m("/api/storefront/self-hosted") {
		t.Error("any single segment must match the parameter")
	}
}

// A parameter stands for exactly one non-empty segment. Without this, "{slug}"
// would drift into the prefix matching that publicExactPaths was introduced to
// eliminate.
func TestPublicGETMatcherParameterIsExactlyOneSegment(t *testing.T) {
	m := matcherFor(t, publicGET("storefront", "/api/storefront/{slug}"))

	for _, path := range []string{
		"/api/storefront",              // too short — the collection is a different route
		"/api/storefront/",             // empty parameter
		"/api/storefront/pro/secrets",  // too long
		"/api/storefronts/pro",         // literal segment differs
		"/api/storefront/pro/{slug}",   // not a path a browser would send, still not a match
		"/internal/api/storefront/pro", // no prefix inheritance from the left
	} {
		if m(path) {
			t.Errorf("%s must not match /api/storefront/{slug}", path)
		}
	}
}

// A non-public or non-GET route must never qualify, parameterized or not —
// this is what keeps the CORS surface to public reads only.
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
