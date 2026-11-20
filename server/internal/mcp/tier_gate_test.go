package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

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
