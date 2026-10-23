package app

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

// tierMCPPlugin is an MCPProvider that records whether the composition reached
// it, so the app-level test can see the gate's decision.
type tierMCPPlugin struct {
	tier       string
	registered bool
}

func (p *tierMCPPlugin) Name() string                      { return "tier-mcp" }
func (p *tierMCPPlugin) Models() []any                     { return nil }
func (p *tierMCPPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (p *tierMCPPlugin) MCPTier() string                   { return p.tier }
func (p *tierMCPPlugin) RegisterMCP(*mcp.Server)           { p.registered = true }

// TestRunMCPAppliesTierGate pins the wiring: the gate set on the App reaches
// the stdio composition, so a tool above the operator's tier is never
// registered. A broken wire here is invisible until an unlicensed build serves
// the tool.
func TestRunMCPAppliesTierGate(t *testing.T) {
	cases := []struct {
		name       string
		tier       string
		gate       plugin.TierGate
		registered bool
	}{
		{"free tool under a denying gate", "", func(string) bool { return false }, true},
		{"paid tool under a nil gate", "pro", nil, true},
		{"paid tool when covered", "pro", func(tier string) bool { return tier == "pro" }, true},
		{"paid tool when uncovered", "pro", func(string) bool { return false }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := bootApp(t)
			p := &tierMCPPlugin{tier: tc.tier}
			a.Use(p)
			a.WithMCPTierGate(tc.gate)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_ = a.RunMCP(ctx)

			if p.registered != tc.registered {
				t.Errorf("RegisterMCP called = %v, want %v", p.registered, tc.registered)
			}
		})
	}
}
