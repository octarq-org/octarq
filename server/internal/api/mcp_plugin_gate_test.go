package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

// gateMCPPlugin is a toggleable feature with one MCP tool. It exists to pin the
// rule that turning a feature off removes its agent tools, not only its routes
// and sidebar entry.
type gateMCPPlugin struct{}

func (p *gateMCPPlugin) Name() string                      { return "gatetool" }
func (p *gateMCPPlugin) Models() []any                     { return nil }
func (p *gateMCPPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (p *gateMCPPlugin) Describe() plugin.Info {
	return plugin.Info{Title: "Gate Tool", EnabledByDefault: true}
}
func (p *gateMCPPlugin) MCPTier() string { return "" }
func (p *gateMCPPlugin) RegisterMCP(srv *mcp.Server) {
	plugin.AddMCPTool(srv, "gatetool", &mcp.Tool{
		Name:        "gatetool__ping",
		Description: "Answer with pong.",
	}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil, nil
	})
}

// mcpToolNames lists the tools one MCP session can see.
func mcpToolNames(t *testing.T, endpoint string) []string {
	t.Helper()
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "gate-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// A workspace that turns a feature off must not reach it over MCP either. The
// HTTP gate is what customers see; the agent tool list is a second door, and a
// disabled plugin left reachable there makes the toggle a half-measure.
func TestMCPToolsFollowTheFeatureToggle(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	h.SetPlugins([]plugin.Plugin{&gateMCPPlugin{}})

	token := "oct_" + strings.Repeat("a", 40)
	if err := db.Create(&models.Token{
		OrgID: 1, Name: "gate", Hash: models.HashToken(token), Prefix: "oct_gate",
	}).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}

	ts := httptest.NewServer(h.mcpStreamHandler())
	defer ts.Close()
	endpoint := ts.URL + "?token=" + token

	if names := mcpToolNames(t, endpoint); !containsName(names, "gatetool__ping") {
		t.Fatalf("tools while enabled = %v, want gatetool__ping", names)
	}

	if err := db.Create(&models.PluginSetting{OrgID: 1, Plugin: "gatetool", Enabled: false}).Error; err != nil {
		t.Fatalf("disable feature: %v", err)
	}
	if names := mcpToolNames(t, endpoint); containsName(names, "gatetool__ping") {
		t.Fatalf("tools while disabled = %v, want no gatetool__ping", names)
	}

	// Another workspace that never toggled keeps the declared default.
	if err := db.Create(&models.Token{
		OrgID: 2, Name: "gate2", Hash: models.HashToken("oct_" + strings.Repeat("b", 40)), Prefix: "oct_gat2",
	}).Error; err != nil {
		t.Fatalf("seed second token: %v", err)
	}
	other := ts.URL + "?token=oct_" + strings.Repeat("b", 40)
	if names := mcpToolNames(t, other); !containsName(names, "gatetool__ping") {
		t.Fatalf("workspace 2 tools = %v, want the default-on tool", names)
	}
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
