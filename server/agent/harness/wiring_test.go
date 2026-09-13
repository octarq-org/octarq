package harness_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/octarq-org/octarq/agent/harness"
	"github.com/octarq-org/octarq/internal/endpoint"
	"github.com/octarq-org/octarq/plugin"
)

// ---------- test types (mirrors plugin/endpoints_test.go echo pattern) ----------

type wiringIn struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type wiringOut struct {
	Reply string `json:"reply"`
	Total int    `json:"total"`
}

// buildEngine creates a minimal Engine populated with test endpoints.
func buildEngine(t *testing.T) *endpoint.Engine {
	t.Helper()
	eng := endpoint.NewEngine()

	// 1. A normal read endpoint.
	readSpec := plugin.EndpointSpec[wiringIn, wiringOut]{
		Name:      "echo_tool",
		Summary:   "Echoes",
		Method:    "GET",
		Path:      "/api/echo",
		ExposeMCP: true,
		Handler: func(ctx context.Context, in wiringIn) (*wiringOut, error) {
			orgID := plugin.OrgIDFromContext(ctx)
			return &wiringOut{
				Reply: fmt.Sprintf("org=%d msg=%s", orgID, in.Message),
				Total: in.Count,
			}, nil
		},
	}
	if err := eng.Register(readSpec); err != nil {
		t.Fatalf("register echo_tool: %v", err)
	}

	// 2. An endpoint requiring approval.
	approvalSpec := plugin.EndpointSpec[wiringIn, wiringOut]{
		Name:            "delete_everything",
		Summary:         "Nuke",
		Method:          "DELETE",
		Path:            "/api/nuke",
		RiskLevel:       plugin.RiskLevelDestructive,
		RequireApproval: true,
		ExposeMCP:       true,
		Handler: func(ctx context.Context, in wiringIn) (*wiringOut, error) {
			return &wiringOut{Reply: "nuked"}, nil
		},
	}
	if err := eng.Register(approvalSpec); err != nil {
		t.Fatalf("register delete_everything: %v", err)
	}

	// 3. A write endpoint without approval.
	writeSpec := plugin.EndpointSpec[wiringIn, wiringOut]{
		Name:      "update_item",
		Summary:   "Update",
		Method:    "PUT",
		Path:      "/api/items",
		ExposeMCP: true,
		Handler: func(ctx context.Context, in wiringIn) (*wiringOut, error) {
			return &wiringOut{Reply: "updated"}, nil
		},
	}
	if err := eng.Register(writeSpec); err != nil {
		t.Fatalf("register update_item: %v", err)
	}

	return eng
}

// ① Unknown tool → executor returns AgentError 404 UNKNOWN_TOOL
func TestRegistryExecutor_UnknownTool(t *testing.T) {
	eng := buildEngine(t)
	exec := harness.NewRegistryExecutor(eng)

	_, err := exec.Execute(context.Background(), 1, "nonexistent", "")
	if err == nil {
		t.Fatal("expected error for unknown tool")
		return
	}
	var ae *plugin.AgentError
	if !errors.As(err, &ae) {
		t.Fatalf("expected AgentError, got %T: %v", err, err)
	}
	if ae.HTTPCode != 404 || ae.Code != "UNKNOWN_TOOL" {
		t.Errorf("unexpected AgentError: code=%d/%s", ae.HTTPCode, ae.Code)
	}
}

// ① Unknown tool → guard returns AgentError 404 UNKNOWN_TOOL with guidance
func TestRiskGuard_UnknownTool(t *testing.T) {
	eng := buildEngine(t)
	g := harness.NewRiskGuard(eng)

	err := g.Allow(context.Background(), 1, "does_not_exist")
	if err == nil {
		t.Fatal("expected error for unknown tool")
		return
	}
	var ae *plugin.AgentError
	if !errors.As(err, &ae) {
		t.Fatalf("expected AgentError, got %T: %v", err, err)
	}
	if ae.HTTPCode != 404 || ae.Code != "UNKNOWN_TOOL" {
		t.Errorf("unexpected AgentError: code=%d/%s", ae.HTTPCode, ae.Code)
	}
	if ae.AgentGuidance == "" {
		t.Error("expected non-empty guidance")
	}
}

// ② Full-chain happy path: JSON args → handler → marshal output
func TestRegistryExecutor_FullChain(t *testing.T) {
	eng := buildEngine(t)
	exec := harness.NewRegistryExecutor(eng)

	// Test with string + int fields to cover number round-trip (陷阱: float64 default).
	argsJSON := `{"message":"hello","count":42}`
	out, err := exec.Execute(context.Background(), 7, "echo_tool", argsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result wiringOut
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result.Reply != "org=7 msg=hello" {
		t.Errorf("unexpected reply: %s", result.Reply)
	}
	if result.Total != 42 {
		t.Errorf("expected Total=42, got %d", result.Total)
	}
}

// ② Empty args → zero-value In
func TestRegistryExecutor_EmptyArgs(t *testing.T) {
	eng := buildEngine(t)
	exec := harness.NewRegistryExecutor(eng)

	out, err := exec.Execute(context.Background(), 1, "echo_tool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result wiringOut
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Reply != "org=1 msg=" {
		t.Errorf("unexpected reply for empty args: %s", result.Reply)
	}
}

// ② "null" args → zero-value In
func TestRegistryExecutor_NullArgs(t *testing.T) {
	eng := buildEngine(t)
	exec := harness.NewRegistryExecutor(eng)

	out, err := exec.Execute(context.Background(), 1, "echo_tool", "null")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result wiringOut
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Reply != "org=1 msg=" {
		t.Errorf("unexpected reply for null args: %s", result.Reply)
	}
}

// ③ Bad JSON → AgentError INVALID_TOOL_ARGS
func TestRegistryExecutor_BadJSON(t *testing.T) {
	eng := buildEngine(t)
	exec := harness.NewRegistryExecutor(eng)

	_, err := exec.Execute(context.Background(), 1, "echo_tool", "{bad json")
	if err == nil {
		t.Fatal("expected error for bad JSON")
		return
	}
	var ae *plugin.AgentError
	if !errors.As(err, &ae) {
		t.Fatalf("expected AgentError, got %T: %v", err, err)
	}
	if ae.HTTPCode != 400 || ae.Code != "INVALID_TOOL_ARGS" {
		t.Errorf("unexpected code: %d/%s", ae.HTTPCode, ae.Code)
	}
}

// ④ RequireApproval tool → guard returns ErrApprovalRequired
func TestRiskGuard_RequireApproval(t *testing.T) {
	eng := buildEngine(t)
	g := harness.NewRiskGuard(eng)

	err := g.Allow(context.Background(), 1, "delete_everything")
	if err == nil {
		t.Fatal("expected error for RequireApproval tool")
		return
	}
	if !errors.Is(err, harness.ErrApprovalRequired) {
		t.Errorf("expected ErrApprovalRequired, got: %v", err)
	}
}

// ⑤ Non-approval tool → guard allows
func TestRiskGuard_AllowsNormalTools(t *testing.T) {
	eng := buildEngine(t)
	g := harness.NewRiskGuard(eng)

	// read tool
	if err := g.Allow(context.Background(), 1, "echo_tool"); err != nil {
		t.Errorf("expected guard to allow echo_tool, got: %v", err)
	}
	// write tool without approval
	if err := g.Allow(context.Background(), 1, "update_item"); err != nil {
		t.Errorf("expected guard to allow update_item, got: %v", err)
	}
}

// ⑥ Org context injection: handler reads OrgIDFromContext correctly
func TestRegistryExecutor_OrgIDInjection(t *testing.T) {
	eng := endpoint.NewEngine()

	type orgEchoIn struct{}
	type orgEchoOut struct {
		OrgID uint `json:"org_id"`
	}

	spec := plugin.EndpointSpec[orgEchoIn, orgEchoOut]{
		Name:    "org_echo",
		Summary: "Returns org ID",
		Method:  "GET",
		Path:    "/api/org-echo",
		Handler: func(ctx context.Context, _ orgEchoIn) (*orgEchoOut, error) {
			return &orgEchoOut{OrgID: plugin.OrgIDFromContext(ctx)}, nil
		},
	}
	if err := eng.Register(spec); err != nil {
		t.Fatalf("register: %v", err)
	}

	exec := harness.NewRegistryExecutor(eng)

	for _, orgID := range []uint{0, 1, 42, 9999} {
		out, err := exec.Execute(context.Background(), orgID, "org_echo", "")
		if err != nil {
			t.Fatalf("orgID=%d: %v", orgID, err)
		}
		var result orgEchoOut
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("orgID=%d unmarshal: %v", orgID, err)
		}
		if result.OrgID != orgID {
			t.Errorf("orgID=%d: handler saw %d", orgID, result.OrgID)
		}
	}
}

// Engine.Lookup correctness
func TestEngine_Lookup(t *testing.T) {
	eng := buildEngine(t)

	ep, ok := eng.Lookup("echo_tool")
	if !ok {
		t.Fatal("expected to find echo_tool")
	}
	if ep.EndpointName() != "echo_tool" {
		t.Errorf("unexpected name: %s", ep.EndpointName())
	}

	_, ok = eng.Lookup("missing")
	if ok {
		t.Error("expected Lookup to return false for missing endpoint")
	}
}
