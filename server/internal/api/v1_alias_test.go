package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV1AliasRejected(t *testing.T) {
	srv, _ := newTestHandler(t)

	for _, path := range []string{"/api/v1/health", "/api/v1/status", "/api/v1/links", "/api/v1/webhook/acme/email/inbound/tok"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404 (no /api/v1/ compatibility shim)", path, rec.Code)
		}
	}
}
