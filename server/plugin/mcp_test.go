package plugin_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestAddMCPTool_ConflictDetection(t *testing.T) {
	impl := &mcp.Implementation{Name: "test", Version: "1.0"}
	opts := &mcp.ServerOptions{}
	srv := mcp.NewServer(impl, opts)

	handler := func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	}

	tool1 := &mcp.Tool{
		Name:        "test_tool_unique",
		Description: "A unique tool",
		InputSchema: map[string]any{"type": "object"},
	}

	// Registering normally should succeed
	plugin.AddMCPTool(srv, "plugin_a", tool1, handler)

	// Registering the exact same tool name from a DIFFERENT plugin should panic
	tool2 := &mcp.Tool{
		Name:        "test_tool_unique",
		Description: "A conflicting tool",
		InputSchema: map[string]any{"type": "object"},
	}

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				msg := r.(string)
				if msg != `mcp: tool conflict: "test_tool_unique" already registered by "plugin_a"` {
					t.Errorf("unexpected panic message: %v", msg)
				}
			}
		}()
		plugin.AddMCPTool(srv, "plugin_b", tool2, handler)
	}()

	if !panicked {
		t.Errorf("expected panic when registering duplicate tool from a different plugin, but it succeeded")
	}

	// Registering the same tool name from the SAME plugin should be allowed
	// (this happens when a new networked instance calls RegisterMCP again)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("unexpected panic when registering same tool from same plugin: %v", r)
			}
		}()
		plugin.AddMCPTool(srv, "plugin_a", tool2, handler)
	}()
}
