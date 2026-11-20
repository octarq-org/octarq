package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/octarq-org/octarq/server/plugin"
)

var ErrApprovalRequired = errors.New("harness: tool requires operator approval")

type EndpointSource interface {
	Lookup(name string) (plugin.Endpoint, bool)
	Endpoints() []plugin.Endpoint
}

type RegistryExecutor struct {
	src EndpointSource
}

func NewRegistryExecutor(src EndpointSource) ToolExecutor {
	return &RegistryExecutor{src: src}
}

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

type riskGuard struct {
	src EndpointSource
}

func NewRiskGuard(src EndpointSource) Guard {
	return &riskGuard{src: src}
}

func (g *riskGuard) Allow(_ context.Context, _ uint, tool, _ string) error {
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
