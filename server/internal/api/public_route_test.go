package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

func TestPublicMetadataBypassesAuth(t *testing.T) {
	h, _ := newHandlerForAdminTest(t)
	mux := h.Routes()
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

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/test/public", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public route: want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/test/protected", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected route: want 401, got %d (body %s)", rec.Code, rec.Body.String())
	}
}
