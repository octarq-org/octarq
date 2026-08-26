package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/octarq-org/octarq/plugin"
)

// ErrApprovalRequired is returned by RiskGuard when the tool's endpoint has
// RequireApproval=true. Channel adapters should translate this into a HITL
// flow rather than treating it as a terminal failure.
//
// P3 前 API 易变.
var ErrApprovalRequired = errors.New("harness: tool requires operator approval")

// EndpointSource is the subset of endpoint.Engine that wiring components need.
// It decouples the harness from the concrete Engine type so both can be
// injected through constructors.
//
// P3 前 API 易变.
type EndpointSource interface {
	Lookup(name string) (plugin.Endpoint, bool)
}

// RegistryExecutor adapts an EndpointSource into the harness ToolExecutor
// interface: it resolves a tool name, injects the org-ID into the context
// (reusing plugin.WithOrgID so handlers read the same key), calls
// ExecuteAgentJSON, and marshals the result back to a JSON string.
//
// P3 前 API 易变.
type RegistryExecutor struct {
	src EndpointSource
}

// NewRegistryExecutor constructs a RegistryExecutor backed by the given source.
func NewRegistryExecutor(src EndpointSource) ToolExecutor {
	return &RegistryExecutor{src: src}
}

// Execute implements ToolExecutor.
func (r *RegistryExecutor) Execute(ctx context.Context, orgID uint, name string, argsJSON string) (string, error) {
	if r.src == nil {
		return "", plugin.NewAgentError(
			404,
			"UNKNOWN_TOOL",
			fmt.Sprintf("tool %q not found", name),
			"Use only tools listed in the system prompt. Call the tool-listing endpoint to see available tools.",
			false,
		)
	}
	ep, ok := r.src.Lookup(name)
	if !ok {
		return "", plugin.NewAgentError(
			404,
			"UNKNOWN_TOOL",
			fmt.Sprintf("tool %q not found", name),
			"Use only tools listed in the system prompt. Call the tool-listing endpoint to see available tools.",
			false,
		)
	}

	// Inject org-ID into context so handlers see it via plugin.OrgIDFromContext.
	ctx = plugin.WithOrgID(ctx, orgID)

	result, err := ep.ExecuteAgentJSON(ctx, argsJSON)
	if err != nil {
		return "", err
	}

	buf, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("harness: marshal tool output: %w", err)
	}
	return string(buf), nil
}

// riskGuard is the concrete Guard implementation that checks endpoint
// existence and approval requirements.
type riskGuard struct {
	src EndpointSource
}

// NewRiskGuard constructs a Guard that vetoes unknown tools and tools that
// require operator approval (returning ErrApprovalRequired for the latter).
//
// The destructive-must-have-approval invariant is enforced at registration
// time (plugin.RegisterEndpoint), so this guard does not replicate that
// business rule.
//
// P3 前 API 易变.
func NewRiskGuard(src EndpointSource) Guard {
	return &riskGuard{src: src}
}

// Allow implements Guard.
func (g *riskGuard) Allow(_ context.Context, _ uint, tool string) error {
	if g.src == nil {
		return plugin.NewAgentError(
			404,
			"UNKNOWN_TOOL",
			fmt.Sprintf("tool %q not found", tool),
			"Use only tools listed in the system prompt. Call the tool-listing endpoint to see available tools.",
			false,
		)
	}
	ep, ok := g.src.Lookup(tool)
	if !ok {
		return plugin.NewAgentError(
			404,
			"UNKNOWN_TOOL",
			fmt.Sprintf("tool %q not found", tool),
			"Use only tools listed in the system prompt. Call the tool-listing endpoint to see available tools.",
			false,
		)
	}
	if ep.EndpointRequireApproval() {
		return fmt.Errorf("%w: %s", ErrApprovalRequired, tool)
	}
	return nil
}
