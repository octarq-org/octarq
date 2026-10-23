package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

// tieredPlugin is an MCPProvider whose tier and registration count are
// observable.
type tieredPlugin struct {
	name  string
	tier  string
	calls int
}

func (p *tieredPlugin) Name() string                      { return p.name }
func (p *tieredPlugin) Models() []any                     { return nil }
func (p *tieredPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (p *tieredPlugin) MCPTier() string                   { return p.tier }
func (p *tieredPlugin) RegisterMCP(*mcp.Server)           { p.calls++ }

// TestRegisterMCPSkipsUncoveredTiers pins the MCP half of the license: a tool
// declared above the operator's tier is never registered, so it cannot be
// listed or called. Without this, an unlicensed binary served every Pro tool
// over stdio even though the same routes answered 402.
func TestRegisterMCPSkipsUncoveredTiers(t *testing.T) {
	cases := []struct {
		name string
		tier string
		gate plugin.TierGate
		want int
	}{
		{"free tool mounts under a denying gate", "", func(string) bool { return false }, 1},
		{"paid tool mounts under a nil gate", "pro", nil, 1},
		{"paid tool mounts when covered", "pro", func(tier string) bool { return tier == "pro" }, 1},
		{"paid tool is skipped when uncovered", "pro", func(string) bool { return false }, 0},
		{"elite tool is skipped by a pro gate", "elite", func(tier string) bool { return tier == "pro" }, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &tieredPlugin{name: "tiered", tier: tc.tier}
			NewServerInstance(nil, 1, []plugin.Plugin{p}, tc.gate)
			if p.calls != tc.want {
				t.Errorf("RegisterMCP calls = %d, want %d", p.calls, tc.want)
			}
		})
	}
}
