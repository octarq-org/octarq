package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
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
		return
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

func TestEndpointSpec_SpecAndExecuteAgentJSON(t *testing.T) {
	spec := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "agent_echo",
		Handler: func(ctx context.Context, input echoIn) (*echoOut, error) {
			if input.Message == "err" {
				return nil, errors.New("echo failure")
			}
			return &echoOut{Reply: "echo:" + input.Message}, nil
		},
	}

	if spec.Spec() == nil {
		t.Errorf("expected non-nil Spec")
	}

	ctx := context.Background()

	// Valid JSON
	res, err := spec.ExecuteAgentJSON(ctx, `{"message":"agent"}`)
	if err != nil || res.(*echoOut).Reply != "echo:agent" {
		t.Fatalf("ExecuteAgentJSON failed: %v, res=%v", err, res)
	}

	// Empty string & null
	resEmpty, errEmpty := spec.ExecuteAgentJSON(ctx, "")
	if errEmpty != nil || resEmpty.(*echoOut).Reply != "echo:" {
		t.Fatalf("ExecuteAgentJSON empty failed: %v, res=%v", errEmpty, resEmpty)
	}
	resNull, errNull := spec.ExecuteAgentJSON(ctx, "null")
	if errNull != nil || resNull.(*echoOut).Reply != "echo:" {
		t.Fatalf("ExecuteAgentJSON null failed: %v, res=%v", errNull, resNull)
	}

	// Invalid JSON
	_, errBadJSON := spec.ExecuteAgentJSON(ctx, "{invalid-json")
	if errBadJSON == nil {
		t.Fatalf("expected error on invalid JSON")
	}
	if ae, ok := plugin.AsAgentError(errBadJSON); !ok || ae.Code != "INVALID_TOOL_ARGS" {
		t.Errorf("expected INVALID_TOOL_ARGS AgentError, got %v", errBadJSON)
	}

	// Nil handler
	nilHandlerSpec := plugin.EndpointSpec[echoIn, echoOut]{Name: "nil_handler"}
	if _, err := nilHandlerSpec.ExecuteAgentJSON(ctx, `{}`); err == nil {
		t.Errorf("expected error on nil handler in ExecuteAgentJSON")
	}
	if _, err := nilHandlerSpec.Execute(ctx, nil); err == nil {
		t.Errorf("expected error on nil handler in Execute")
	}

	// Execute with nil input
	resNilInput, errNilInput := spec.Execute(ctx, nil)
	if errNilInput != nil || resNilInput.(*echoOut).Reply != "echo:" {
		t.Errorf("Execute with nil input failed: %v", errNilInput)
	}
}

func TestRegisterTenantView(t *testing.T) {
	// Nil context
	plugin.RegisterTenantView(nil, plugin.TenantView{Name: "tenant_links"})

	// Context with nil callback
	ctx := &plugin.Context{}
	plugin.RegisterTenantView(ctx, plugin.TenantView{Name: "tenant_links"})

	// Context with callback
	var registered plugin.TenantView
	ctx.RegisterTenantView = func(v plugin.TenantView) {
		registered = v
	}
	plugin.RegisterTenantView(ctx, plugin.TenantView{Name: "tenant_links"})
	if registered.Name != "tenant_links" {
		t.Errorf("expected tenant_links, got %s", registered.Name)
	}
}

func TestPlugin_StartSpan(t *testing.T) {
	ctx, span := plugin.StartSpan(context.Background(), "test-plugin", "test-op")
	if span == nil || ctx == nil {
		t.Errorf("expected valid ctx and span from StartSpan")
	}
	span.End()
}
