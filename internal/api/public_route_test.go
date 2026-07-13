package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// TestPublicMetadataBypassesAuth verifies the sanctioned plugin mechanism for
// self-authenticating routes: an operation registered with
// Metadata["public"] = true skips the core dashboard-auth middleware, while an
// otherwise identical operation without it still 401s when unauthenticated.
//
// A second agent wires octarq-pro buyer routes to this exact contract — the
// "public" boolean metadata key on huma.Operation.
func TestPublicMetadataBypassesAuth(t *testing.T) {
	h, _ := newHandlerForAdminTest(t)
	mux := h.Routes() // builds and populates h.humaAPI
	api := h.Huma()

	type out struct {
		Body struct {
			OK bool `json:"ok"`
		}
	}
	handler := func(ctx context.Context, _ *struct{}) (*out, error) {
		o := &out{}
		o.Body.OK = true
		return o, nil
	}

	huma.Register(api, huma.Operation{
		OperationID: "test-public",
		Method:      "GET",
		Path:        "/api/test/public",
		Metadata:    map[string]any{"public": true},
	}, handler)

	huma.Register(api, huma.Operation{
		OperationID: "test-protected",
		Method:      "GET",
		Path:        "/api/test/protected",
	}, handler)

	// Public operation: reachable without any credentials.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/test/public", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public route: want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}

	// Protected operation: 401 when unauthenticated.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/test/protected", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected route: want 401, got %d (body %s)", rec.Code, rec.Body.String())
	}
}
