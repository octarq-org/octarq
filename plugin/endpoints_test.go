package plugin_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
)

type echoIn struct {
	Message string `json:"message"`
}

type echoOut struct {
	Reply string `json:"reply"`
}

func TestEndpointSpec_Execute(t *testing.T) {
	spec := plugin.EndpointSpec[echoIn, echoOut]{
		Name:        "echo",
		Summary:     "Echoes a message",
		Description: "Returns the input message in reply field",
		Method:      "POST",
		Path:        "/api/echo",
		RequireAuth: true,
		RequireRole: []string{"admin"},
		ExposeMCP:   true,
		Handler: func(ctx context.Context, input echoIn) (*echoOut, error) {
			if input.Message == "fail" {
				return nil, errors.New("forced failure")
			}
			return &echoOut{Reply: "echo: " + input.Message}, nil
		},
	}

	if spec.EndpointName() != "echo" {
		t.Errorf("expected Name 'echo', got %q", spec.EndpointName())
	}
	if spec.EndpointMethod() != "POST" {
		t.Errorf("expected Method 'POST', got %q", spec.EndpointMethod())
	}
	if spec.EndpointPath() != "/api/echo" {
		t.Errorf("expected Path '/api/echo', got %q", spec.EndpointPath())
	}
	if !spec.EndpointRequireAuth() {
		t.Errorf("expected RequireAuth true")
	}
	if len(spec.EndpointRequireRole()) != 1 || spec.EndpointRequireRole()[0] != "admin" {
		t.Errorf("expected RequireRole ['admin'], got %v", spec.EndpointRequireRole())
	}
	if !spec.EndpointExposeMCP() {
		t.Errorf("expected ExposeMCP true")
	}
	if spec.EndpointRequireApproval() {
		t.Errorf("expected RequireApproval false by default")
	}

	// Success case
	res, err := spec.Execute(context.Background(), echoIn{Message: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := res.(*echoOut)
	if !ok || out.Reply != "echo: hello" {
		t.Fatalf("expected 'echo: hello', got %+v", res)
	}

	// Pointer input conversion
	resPtr, err := spec.Execute(context.Background(), &echoIn{Message: "ptr"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outPtr, ok := resPtr.(*echoOut)
	if !ok || outPtr.Reply != "echo: ptr" {
		t.Fatalf("expected 'echo: ptr', got %+v", resPtr)
	}

	// Error case
	_, err = spec.Execute(context.Background(), echoIn{Message: "fail"})
	if err == nil || err.Error() != "forced failure" {
		t.Fatalf("expected 'forced failure' error, got %v", err)
	}

	// Incompatible type error
	_, err = spec.Execute(context.Background(), 12345)
	if err == nil {
		t.Fatalf("expected error for incompatible input type")
	}
}

func TestRegisterEndpoint_NilContext(t *testing.T) {
	spec := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "echo",
	}
	// Calling with nil Context should return nil without panic
	if err := plugin.RegisterEndpoint[echoIn, echoOut](nil, spec); err != nil {
		t.Errorf("expected nil error on nil context, got %v", err)
	}

	// Calling with uninitialized RegisterEndpoint func
	ctx := &plugin.Context{}
	if err := plugin.RegisterEndpoint(ctx, spec); err != nil {
		t.Errorf("expected nil error on uninitialized RegisterEndpoint, got %v", err)
	}

	// Calling with registered handler
	var registered any
	ctx.RegisterEndpoint = func(s any) error {
		registered = s
		return nil
	}
	if err := plugin.RegisterEndpoint(ctx, spec); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if registered == nil {
		t.Errorf("expected spec to be passed to RegisterEndpoint")
	}
}

func TestEndpointSpec_Validate(t *testing.T) {
	// 1. Valid /api/links
	specValid := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "valid_endpoint",
		Path: "/api/links",
	}
	if err := specValid.Validate(); err != nil {
		t.Errorf("expected valid for /api/links, got: %v", err)
	}

	// 2. Valid /api (bare prefix boundary)
	specBare := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "bare_api",
		Path: "/api",
	}
	if err := specBare.Validate(); err != nil {
		t.Errorf("expected valid for /api, got: %v", err)
	}

	// 3. Empty Name
	specEmptyName := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "",
		Path: "/api/links",
	}
	if err := specEmptyName.Validate(); err == nil {
		t.Errorf("expected error for empty name, got nil")
	}

	// 4. Invalid Path /foo
	specInvalidPath := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "invalid_path",
		Path: "/foo",
	}
	if err := specInvalidPath.Validate(); err == nil {
		t.Errorf("expected error for path /foo, got nil")
	}
}

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

func TestDefaultRiskLevel(t *testing.T) {
	tests := []struct {
		method   string
		expected string
	}{
		{"GET", plugin.RiskLevelRead},
		{"get", plugin.RiskLevelRead},
		{"HEAD", plugin.RiskLevelRead},
		{"head", plugin.RiskLevelRead},
		{"POST", plugin.RiskLevelWrite},
		{"post", plugin.RiskLevelWrite},
		{"PUT", plugin.RiskLevelWrite},
		{"put", plugin.RiskLevelWrite},
		{"PATCH", plugin.RiskLevelWrite},
		{"patch", plugin.RiskLevelWrite},
		{"DELETE", plugin.RiskLevelWrite},
		{"delete", plugin.RiskLevelWrite},
		{"", plugin.RiskLevelWrite},
		{"OPTIONS", plugin.RiskLevelWrite},
		{"UNKNOWN", plugin.RiskLevelWrite},
	}

	for _, tc := range tests {
		got := plugin.DefaultRiskLevel(tc.method)
		if got != tc.expected {
			t.Errorf("DefaultRiskLevel(%q) = %q, expected %q", tc.method, got, tc.expected)
		}
	}
}

func TestEndpointSpec_EffectiveRisk(t *testing.T) {
	// 1. Inferred from GET
	specGet := plugin.EndpointSpec[echoIn, echoOut]{
		Method: "GET",
	}
	if specGet.EffectiveRisk() != plugin.RiskLevelRead {
		t.Errorf("expected %q for GET, got %q", plugin.RiskLevelRead, specGet.EffectiveRisk())
	}

	// 2. Inferred from POST (explicit or default)
	specPost := plugin.EndpointSpec[echoIn, echoOut]{
		Method: "POST",
	}
	if specPost.EffectiveRisk() != plugin.RiskLevelWrite {
		t.Errorf("expected %q for POST, got %q", plugin.RiskLevelWrite, specPost.EffectiveRisk())
	}
	specEmptyMethod := plugin.EndpointSpec[echoIn, echoOut]{}
	if specEmptyMethod.EffectiveRisk() != plugin.RiskLevelWrite {
		t.Errorf("expected %q for default method, got %q", plugin.RiskLevelWrite, specEmptyMethod.EffectiveRisk())
	}

	// 3. Inferred from DELETE
	specDelete := plugin.EndpointSpec[echoIn, echoOut]{
		Method: "DELETE",
	}
	if specDelete.EffectiveRisk() != plugin.RiskLevelWrite {
		t.Errorf("expected %q for DELETE, got %q", plugin.RiskLevelWrite, specDelete.EffectiveRisk())
	}

	// 4. Explicit override: read on POST
	specOverrideRead := plugin.EndpointSpec[echoIn, echoOut]{
		Method:    "POST",
		RiskLevel: plugin.RiskLevelRead,
	}
	if specOverrideRead.EffectiveRisk() != plugin.RiskLevelRead {
		t.Errorf("expected explicit %q on POST, got %q", plugin.RiskLevelRead, specOverrideRead.EffectiveRisk())
	}

	// 5. Explicit override: write on GET
	specOverrideWrite := plugin.EndpointSpec[echoIn, echoOut]{
		Method:    "GET",
		RiskLevel: plugin.RiskLevelWrite,
	}
	if specOverrideWrite.EffectiveRisk() != plugin.RiskLevelWrite {
		t.Errorf("expected explicit %q on GET, got %q", plugin.RiskLevelWrite, specOverrideWrite.EffectiveRisk())
	}

	// 6. Explicit override: destructive on DELETE
	specOverrideDestructive := plugin.EndpointSpec[echoIn, echoOut]{
		Method:    "DELETE",
		RiskLevel: plugin.RiskLevelDestructive,
	}
	if specOverrideDestructive.EffectiveRisk() != plugin.RiskLevelDestructive {
		t.Errorf("expected explicit %q on DELETE, got %q", plugin.RiskLevelDestructive, specOverrideDestructive.EffectiveRisk())
	}

	// 7. Invalid explicit value returns empty string
	specInvalidRisk := plugin.EndpointSpec[echoIn, echoOut]{
		Method:    "POST",
		RiskLevel: "dangerous",
	}
	if specInvalidRisk.EffectiveRisk() != "" {
		t.Errorf("expected empty string for invalid RiskLevel, got %q", specInvalidRisk.EffectiveRisk())
	}
}

func TestRegisterEndpoint_RiskValidation(t *testing.T) {
	ctx := &plugin.Context{
		RegisterEndpoint: func(spec any) error { return nil },
	}

	// 1. Valid endpoint with default (empty) RiskLevel
	specDefault := plugin.EndpointSpec[echoIn, echoOut]{
		Name:   "default_risk",
		Method: "POST",
		Path:   "/api/default",
	}
	if err := plugin.RegisterEndpoint(ctx, specDefault); err != nil {
		t.Errorf("expected no error for default risk, got %v", err)
	}

	// 2. Valid endpoint with explicit valid RiskLevels
	for _, lvl := range []string{plugin.RiskLevelRead, plugin.RiskLevelWrite} {
		specValid := plugin.EndpointSpec[echoIn, echoOut]{
			Name:      "valid_" + lvl,
			Method:    "POST",
			Path:      "/api/" + lvl,
			RiskLevel: lvl,
		}
		if err := plugin.RegisterEndpoint(ctx, specValid); err != nil {
			t.Errorf("expected no error for risk %q, got %v", lvl, err)
		}
	}

	// 3. Valid destructive endpoint with RequireApproval=true
	specDestructiveValid := plugin.EndpointSpec[echoIn, echoOut]{
		Name:            "destroy_all",
		Method:          "DELETE",
		Path:            "/api/all",
		RiskLevel:       plugin.RiskLevelDestructive,
		RequireApproval: true,
	}
	if err := plugin.RegisterEndpoint(ctx, specDestructiveValid); err != nil {
		t.Errorf("expected no error for destructive endpoint with RequireApproval=true, got %v", err)
	}

	// 4. Invalid RiskLevel: must be rejected
	specInvalidRisk := plugin.EndpointSpec[echoIn, echoOut]{
		Name:      "invalid_risk",
		Method:    "POST",
		Path:      "/api/invalid",
		RiskLevel: "super-risky",
	}
	if err := plugin.RegisterEndpoint(ctx, specInvalidRisk); err == nil {
		t.Errorf("expected error for invalid RiskLevel, got nil")
	}

	// 5. Destructive without RequireApproval: must be rejected
	specDestructiveNoApproval := plugin.EndpointSpec[echoIn, echoOut]{
		Name:            "destroy_no_approval",
		Method:          "DELETE",
		Path:            "/api/danger",
		RiskLevel:       plugin.RiskLevelDestructive,
		RequireApproval: false,
	}
	if err := plugin.RegisterEndpoint(ctx, specDestructiveNoApproval); err == nil {
		t.Errorf("expected error for destructive endpoint without RequireApproval, got nil")
	}

	// 6. Validation occurs even if ctx is nil (fail-fast)
	if err := plugin.RegisterEndpoint[echoIn, echoOut](nil, specInvalidRisk); err == nil {
		t.Errorf("expected error on nil ctx for invalid RiskLevel, got nil")
	}
	if err := plugin.RegisterEndpoint[echoIn, echoOut](nil, specDestructiveNoApproval); err == nil {
		t.Errorf("expected error on nil ctx for destructive endpoint without RequireApproval, got nil")
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
			if foundTool == nil {
				t.Fatalf("tool %q not registered", tc.spec.Name)
			}
			if foundTool.Description != tc.expectedDesc {
				t.Errorf("tool description = %q, expected %q", foundTool.Description, tc.expectedDesc)
			}
		})
	}
}

func TestEndpointSpec_EndpointRequireApproval(t *testing.T) {
	specFalse := plugin.EndpointSpec[echoIn, echoOut]{
		RequireApproval: false,
	}
	if specFalse.EndpointRequireApproval() != false {
		t.Errorf("expected false, got true")
	}

	specTrue := plugin.EndpointSpec[echoIn, echoOut]{
		RequireApproval: true,
	}
	if specTrue.EndpointRequireApproval() != true {
		t.Errorf("expected true, got false")
	}

	// Verify interface fulfillment
	var ep plugin.Endpoint = specTrue
	if !ep.EndpointRequireApproval() {
		t.Errorf("expected Endpoint interface to return true for EndpointRequireApproval")
	}
}
