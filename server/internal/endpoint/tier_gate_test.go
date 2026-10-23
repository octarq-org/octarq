package endpoint_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/endpoint"
	"github.com/octarq-org/octarq/server/plugin"
)

// tieredEndpoint is an EndpointSpec whose registration is observable, so a test
// can tell whether MountMCP reached it.
type tieredEndpoint struct {
	plugin.EndpointSpec[struct{}, struct{}]
	tier       string
	exposeMCP  bool
	registered int
}

func (e *tieredEndpoint) EndpointExposeMCP() bool                   { return e.exposeMCP }
func (e *tieredEndpoint) EndpointTier() string                      { return e.tier }
func (e *tieredEndpoint) RegisterMCP(*mcp.Server) error             { e.registered++; return nil }
func (e *tieredEndpoint) Execute(context.Context, any) (any, error) { return nil, nil }

func TestMountMCPRespectsTierGate(t *testing.T) {
	cases := []struct {
		name string
		tier string
		gate plugin.TierGate
		want int
	}{
		{"free endpoint mounts without a gate", "", nil, 1},
		{"paid endpoint mounts under a nil gate", "pro", nil, 1},
		{"paid endpoint mounts when covered", "pro", func(tier string) bool { return tier == "pro" }, 1},
		{"paid endpoint is skipped when uncovered", "pro", func(string) bool { return false }, 0},
		{"elite endpoint is skipped by a pro gate", "elite", func(tier string) bool { return tier == "pro" }, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng := endpoint.NewEngine()
			ep := &tieredEndpoint{tier: tc.tier, exposeMCP: true}
			if err := eng.Register(ep); err != nil {
				t.Fatalf("Register: %v", err)
			}
			srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			if err := eng.MountMCP(srv, tc.gate); err != nil {
				t.Fatalf("MountMCP: %v", err)
			}
			if ep.registered != tc.want {
				t.Errorf("RegisterMCP calls = %d, want %d", ep.registered, tc.want)
			}
		})
	}
}
