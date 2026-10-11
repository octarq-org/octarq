package plugin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestEndpointSpec_RegisterHTTP_RejectsInvalid(t *testing.T) {
	// When api == nil, RegisterHTTP returns nil without checking Validate
	specInvalid := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "invalid",
		Path: "/foo",
	}
	if err := specInvalid.RegisterHTTP(nil, plugin.HTTPOptions{}); err != nil {
		t.Errorf("expected nil error when api is nil, got: %v", err)
	}

	router := http.NewServeMux()
	api := humago.New(router, huma.DefaultConfig("Test API", "1.0.0"))

	// Invalid Path: must fail and not register
	if err := specInvalid.RegisterHTTP(api, plugin.HTTPOptions{}); err == nil {
		t.Error("expected error when registering endpoint with invalid path /foo, got nil")
	}

	// Empty Name: must fail and not register
	specEmptyName := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "",
		Path: "/api/test",
	}
	if err := specEmptyName.RegisterHTTP(api, plugin.HTTPOptions{}); err == nil {
		t.Error("expected error when registering endpoint with empty name, got nil")
	}

	// Valid endpoint: succeeds
	specValid := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "valid",
		Path: "/api/test-valid",
		Handler: func(ctx context.Context, input echoIn) (*echoOut, error) {
			return &echoOut{Reply: "ok"}, nil
		},
	}
	if err := specValid.RegisterHTTP(api, plugin.HTTPOptions{}); err != nil {
		t.Errorf("expected nil error for valid endpoint registration, got: %v", err)
	}
}

func TestEndpointSpec_RegisterMCP_RiskSuffix(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name           string
		spec           plugin.EndpointSpec[echoIn, echoOut]
		expectedSuffix string
		expectedDesc   string
	}{
		{
			name: "Inferred GET read",
			spec: plugin.EndpointSpec[echoIn, echoOut]{
				Name:        "get_data",
				Description: "Fetches information",
				Method:      "GET",
				ExposeMCP:   true,
				Handler:     func(ctx context.Context, in echoIn) (*echoOut, error) { return &echoOut{}, nil },
			},
			expectedDesc: "Fetches information [risk: read]",
		},
		{
			name: "Inferred POST write",
			spec: plugin.EndpointSpec[echoIn, echoOut]{
				Name:        "create_data",
				Description: "Creates a resource",
				Method:      "POST",
				ExposeMCP:   true,
				Handler:     func(ctx context.Context, in echoIn) (*echoOut, error) { return &echoOut{}, nil },
			},
			expectedDesc: "Creates a resource [risk: write]",
		},
		{
			name: "Destructive with approval",
			spec: plugin.EndpointSpec[echoIn, echoOut]{
				Name:            "delete_all",
				Description:     "Deletes all resources",
				Method:          "DELETE",
				RiskLevel:       plugin.RiskLevelDestructive,
				RequireApproval: true,
				ExposeMCP:       true,
				Handler:         func(ctx context.Context, in echoIn) (*echoOut, error) { return &echoOut{}, nil },
			},
			expectedDesc: "Deletes all resources [risk: destructive, approval required]",
		},
		{
			name: "Write with approval required",
			spec: plugin.EndpointSpec[echoIn, echoOut]{
				Name:            "update_critical",
				Summary:         "Updates critical settings",
				Method:          "PUT",
				RequireApproval: true,
				ExposeMCP:       true,
				Handler:         func(ctx context.Context, in echoIn) (*echoOut, error) { return &echoOut{}, nil },
			},
			expectedDesc: "Updates critical settings [risk: write, approval required]",
		},
		{
			name: "Empty desc and summary",
			spec: plugin.EndpointSpec[echoIn, echoOut]{
				Name:      "bare_tool",
				Method:    "POST",
				ExposeMCP: true,
				Handler:   func(ctx context.Context, in echoIn) (*echoOut, error) { return &echoOut{}, nil },
			},
			expectedDesc: "[risk: write]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test-srv", Version: "1.0"}, nil)
			client := mcp.NewClient(&mcp.Implementation{Name: "test-cli", Version: "1.0"}, nil)
			sTrans, cTrans := mcp.NewInMemoryTransports()

			sSession, err := server.Connect(ctx, sTrans, nil)
			if err != nil {
				t.Fatalf("server connect: %v", err)
			}
			defer sSession.Close()

			cSession, err := client.Connect(ctx, cTrans, nil)
			if err != nil {
				t.Fatalf("client connect: %v", err)
			}
			defer cSession.Close()

			if err := tc.spec.RegisterMCP(server); err != nil {
				t.Fatalf("RegisterMCP: %v", err)
			}

			toolsRes, err := cSession.ListTools(ctx, nil)
			if err != nil {
				t.Fatalf("ListTools: %v", err)
			}

			var foundTool *mcp.Tool
			for _, tool := range toolsRes.Tools {
				if tool.Name == tc.spec.Name {
					foundTool = tool
					break
				}
			}
			if foundTool == nil { //nolint:staticcheck // SA5011 false positive: t.Fatalf above is noreturn
				t.Fatalf("tool %q not registered", tc.spec.Name)
				return
			}
			if foundTool.Description != tc.expectedDesc { //nolint:staticcheck // SA5011 false positive: t.Fatalf above is noreturn
				t.Errorf("tool description = %q, expected %q", foundTool.Description, tc.expectedDesc)
			}
		})
	}
}

func TestEndpointSpec_RegisterHTTP_And_MCP_Execution(t *testing.T) {
	// POST endpoint
	specPost := plugin.EndpointSpec[echoIn, echoOut]{
		Name:        "post_echo",
		Method:      "POST",
		Path:        "/api/post-echo",
		RequireAuth: true,
		RequireRole: []string{"admin"},
		ExposeMCP:   true,
		Handler: func(ctx context.Context, input echoIn) (*echoOut, error) {
			if input.Message == "ae" {
				return nil, plugin.NewAgentError(400, "BAD_MSG", "bad msg", "fix it", false)
			}
			if input.Message == "err" {
				return nil, errors.New("internal err")
			}
			if input.Message == "nil" {
				return nil, nil
			}
			return &echoOut{Reply: "post:" + input.Message}, nil
		},
	}

	type queryIn struct {
		Message string `query:"message" json:"message"`
	}

	// GET endpoint
	specGet := plugin.EndpointSpec[queryIn, echoOut]{
		Name:        "get_echo",
		Method:      "GET",
		Path:        "/api/get-echo",
		RequireAuth: true,
		RequireRole: []string{"admin"},
		ExposeMCP:   true,
		Handler: func(ctx context.Context, input queryIn) (*echoOut, error) {
			if input.Message == "ae" {
				return nil, plugin.NewAgentError(403, "FORBIDDEN_GET", "forbidden", "", false)
			}
			if input.Message == "err" {
				return nil, errors.New("get err")
			}
			if input.Message == "nil" {
				return nil, nil
			}
			return &echoOut{Reply: "get:" + input.Message}, nil
		},
	}

	router := http.NewServeMux()
	api := humago.New(router, huma.DefaultConfig("Test API", "1.0.0"))

	opts := plugin.HTTPOptions{
		RequireAuth: func(ctx context.Context) (uint, error) {
			return 1, nil
		},
		RequireRole: func(ctx context.Context, roles []string) error {
			return nil
		},
	}

	if err := specPost.RegisterHTTP(api, opts); err != nil {
		t.Fatalf("RegisterHTTP POST failed: %v", err)
	}
	if err := specGet.RegisterHTTP(api, opts); err != nil {
		t.Fatalf("RegisterHTTP GET failed: %v", err)
	}

	// Test POST success
	body, _ := json.Marshal(map[string]any{"message": "hello"})
	req := httptest.NewRequest("POST", "/api/post-echo", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("POST expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Test POST AgentError
	bodyAE, _ := json.Marshal(map[string]any{"message": "ae"})
	reqAE := httptest.NewRequest("POST", "/api/post-echo", bytes.NewReader(bodyAE))
	reqAE.Header.Set("Content-Type", "application/json")
	wAE := httptest.NewRecorder()
	router.ServeHTTP(wAE, reqAE)
	if wAE.Code != http.StatusBadRequest {
		t.Errorf("POST expected 400 for AgentError, got %d", wAE.Code)
	}

	// Test POST nil output
	bodyNil, _ := json.Marshal(map[string]any{"message": "nil"})
	reqNil := httptest.NewRequest("POST", "/api/post-echo", bytes.NewReader(bodyNil))
	reqNil.Header.Set("Content-Type", "application/json")
	wNil := httptest.NewRecorder()
	router.ServeHTTP(wNil, reqNil)
	if wNil.Code != http.StatusOK {
		t.Errorf("POST nil output expected 200, got %d", wNil.Code)
	}

	// Test GET success
	reqGet := httptest.NewRequest("GET", "/api/get-echo?message=hello", nil)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Errorf("GET expected 200, got %d: %s", wGet.Code, wGet.Body.String())
	}

	// Test GET AgentError
	reqGetAE := httptest.NewRequest("GET", "/api/get-echo?message=ae", nil)
	wGetAE := httptest.NewRecorder()
	router.ServeHTTP(wGetAE, reqGetAE)
	if wGetAE.Code != http.StatusForbidden {
		t.Errorf("GET expected 403 for AgentError, got %d", wGetAE.Code)
	}

	// Test GET nil output
	reqGetNil := httptest.NewRequest("GET", "/api/get-echo?message=nil", nil)
	wGetNil := httptest.NewRecorder()
	router.ServeHTTP(wGetNil, reqGetNil)
	if wGetNil.Code != http.StatusOK {
		t.Errorf("GET nil output expected 200, got %d", wGetNil.Code)
	}

	// Test MCP tool invocation via client
	ctx := context.Background()
	mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	if err := specPost.RegisterMCP(mcpSrv); err != nil {
		t.Fatalf("RegisterMCP failed: %v", err)
	}
	mcpCli := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0"}, nil)
	sT, cT := mcp.NewInMemoryTransports()
	sSes, _ := mcpSrv.Connect(ctx, sT, nil)
	defer sSes.Close()
	cSes, _ := mcpCli.Connect(ctx, cT, nil)
	defer cSes.Close()

	callRes, err := cSes.CallTool(ctx, &mcp.CallToolParams{
		Name:      "post_echo",
		Arguments: map[string]any{"message": "mcp-msg"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if len(callRes.Content) == 0 {
		t.Fatalf("CallTool returned empty content")
	}
}
