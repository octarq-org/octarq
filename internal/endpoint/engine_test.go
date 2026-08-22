package endpoint_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/internal/endpoint"
	"github.com/octarq-org/octarq/plugin"
)

type createItemIn struct {
	Title string `json:"title" doc:"Item title" maxLength:"50"`
	Count int    `json:"count" doc:"Item count" minimum:"1"`
}

type createItemOut struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

func TestEngine_DualEndpoint_HTTPAndMCP(t *testing.T) {
	eng := endpoint.NewEngine()

	spec := plugin.EndpointSpec[createItemIn, createItemOut]{
		Name:        "create_item",
		Summary:     "Create an item",
		Description: "Creates a new item and returns its ID",
		Method:      "POST",
		Path:        "/api/items",
		RequireAuth: true,
		RequireRole: []string{"admin"},
		ExposeMCP:   true,
		Handler: func(ctx context.Context, in createItemIn) (*createItemOut, error) {
			orgID := plugin.OrgIDFromContext(ctx)
			if orgID == 0 {
				return nil, plugin.NewAgentError(401, "UNAUTHORIZED", "unauthorized", "Please sign in", false)
			}
			if in.Title == "error" {
				return nil, plugin.NewAgentError(400, "BAD_TITLE", "invalid title 'error'", "Please use another title", false)
			}
			return &createItemOut{
				ID:    fmt.Sprintf("item-%d-1", orgID),
				Title: in.Title,
				Count: in.Count,
			}, nil
		},
	}

	if err := eng.Register(spec); err != nil {
		t.Fatalf("Register endpoint: %v", err)
	}

	// --- 1. Test HTTP (Huma) endpoint ---
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))

	opts := endpoint.HTTPOptions{
		RequireAuth: func(ctx context.Context) (uint, error) {
			orgID := plugin.OrgIDFromContext(ctx)
			if orgID == 0 {
				return 0, huma.Error401Unauthorized("unauthorized")
			}
			return orgID, nil
		},
		RequireRole: func(ctx context.Context, roles []string) error {
			if len(roles) > 0 && roles[0] == "superadmin_only" {
				return huma.Error403Forbidden("forbidden: insufficient role")
			}
			return nil
		},
	}

	if err := eng.MountHTTP(humaAPI, opts); err != nil {
		t.Fatalf("MountHTTP: %v", err)
	}

	// 1a. Unauthenticated HTTP request -> 401
	body, _ := json.Marshal(map[string]any{"title": "Test", "count": 5})
	req := httptest.NewRequest("POST", "/api/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated HTTP request, got %d", w.Code)
	}

	// 1b. Authenticated HTTP request -> 200 OK
	reqAuth := httptest.NewRequest("POST", "/api/items", bytes.NewReader(body))
	reqAuth.Header.Set("Content-Type", "application/json")
	reqAuth = reqAuth.WithContext(plugin.WithOrgID(reqAuth.Context(), 42))
	wAuth := httptest.NewRecorder()
	mux.ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated HTTP request, got %d: %s", wAuth.Code, wAuth.Body.String())
	}

	var httpResp struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(wAuth.Body.Bytes(), &httpResp); err != nil {
		t.Fatalf("unmarshal HTTP response: %v", err)
	}
	if httpResp.ID != "item-42-1" || httpResp.Title != "Test" || httpResp.Count != 5 {
		t.Errorf("unexpected HTTP response: %+v", httpResp)
	}

	// 1c. HTTP error mapping (AgentError -> HTTP Status)
	errBody, _ := json.Marshal(map[string]any{"title": "error", "count": 1})
	reqErr := httptest.NewRequest("POST", "/api/items", bytes.NewReader(errBody))
	reqErr.Header.Set("Content-Type", "application/json")
	reqErr = reqErr.WithContext(plugin.WithOrgID(reqErr.Context(), 42))
	wErr := httptest.NewRecorder()
	mux.ServeHTTP(wErr, reqErr)
	if wErr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for AgentError BAD_TITLE, got %d", wErr.Code)
	}

	// 1d. HTTP schema validation (minimum: 1)
	invalidBody, _ := json.Marshal(map[string]any{"title": "Test", "count": 0})
	reqInvalid := httptest.NewRequest("POST", "/api/items", bytes.NewReader(invalidBody))
	reqInvalid.Header.Set("Content-Type", "application/json")
	reqInvalid = reqInvalid.WithContext(plugin.WithOrgID(reqInvalid.Context(), 42))
	wInvalid := httptest.NewRecorder()
	mux.ServeHTTP(wInvalid, reqInvalid)
	if wInvalid.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid count=0, got %d", wInvalid.Code)
	}

	// --- 2. Test MCP Tool endpoint ---
	mcpImpl := &mcp.Implementation{Name: "test-mcp", Version: "1.0.0"}
	mcpServer := mcp.NewServer(mcpImpl, nil)

	if err := eng.MountMCP(mcpServer); err != nil {
		t.Fatalf("MountMCP: %v", err)
	}

	// Verify tool registration
	endpoints := eng.Endpoints()
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}
	if endpoints[0].EndpointName() != "create_item" {
		t.Errorf("expected endpoint name 'create_item', got %q", endpoints[0].EndpointName())
	}
}

func TestEngine_RoleGating_HTTP(t *testing.T) {
	eng := endpoint.NewEngine()

	spec := plugin.EndpointSpec[createItemIn, createItemOut]{
		Name:        "admin_item",
		Method:      "POST",
		Path:        "/api/admin/items",
		RequireAuth: true,
		RequireRole: []string{"superadmin"},
		Handler: func(ctx context.Context, in createItemIn) (*createItemOut, error) {
			return &createItemOut{ID: "admin-1", Title: in.Title}, nil
		},
	}
	_ = eng.Register(spec)

	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))

	opts := endpoint.HTTPOptions{
		RequireAuth: func(ctx context.Context) (uint, error) {
			return 1, nil
		},
		RequireRole: func(ctx context.Context, roles []string) error {
			// Member is refused
			return huma.Error403Forbidden("role superadmin required")
		},
	}
	_ = eng.MountHTTP(humaAPI, opts)

	body, _ := json.Marshal(map[string]any{"title": "Test", "count": 1})
	req := httptest.NewRequest("POST", "/api/admin/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for insufficient role, got %d", w.Code)
	}
}

func TestEngine_NonEndpointRegistration(t *testing.T) {
	eng := endpoint.NewEngine()
	err := eng.Register("not an endpoint")
	if err == nil || !strings.Contains(err.Error(), "must implement plugin.Endpoint") {
		t.Errorf("expected error for non-endpoint registration, got %v", err)
	}
}
