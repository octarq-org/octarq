package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// newStdioServer builds the stdio MCP server or fails the test.
func newStdioServer(t *testing.T, gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, gate plugin.TierGate) *mcp.Server {
	t.Helper()
	srv, err := NewServerInstance(context.Background(), gdb, orgID, plugins, gate)
	if err != nil {
		t.Fatalf("NewServerInstance: %v", err)
	}
	return srv
}

// refusingPlugin refuses to start, and records whether its MCP tools were
// registered anyway.
type refusingPlugin struct {
	mounted bool
	tools   bool
}

func (p *refusingPlugin) Name() string                      { return "refusing" }
func (p *refusingPlugin) Models() []any                     { return nil }
func (p *refusingPlugin) Mount(plugin.Mux, *plugin.Context) { p.mounted = true }
func (p *refusingPlugin) Validate(context.Context, plugin.Host) error {
	return errors.New("startup check failed: missing credential")
}
func (p *refusingPlugin) RegisterMCP(*mcp.Server) { p.tools = true }

// TestNewServerInstanceRefusesToBuildOnFailedValidation is the point of this
// PR: `octarq mcp` is its own composition root and never runs app.Run, so it
// used to mount a plugin that refuses to start over HTTP and register its tools
// anyway. A plugin's refusal must hold on every surface.
func TestNewServerInstanceRefusesToBuildOnFailedValidation(t *testing.T) {
	p := &refusingPlugin{}
	srv, err := NewServerInstance(context.Background(), nil, 1, []plugin.Plugin{p}, nil)
	if err == nil {
		t.Fatal("expected the validation failure to abort the MCP server build")
	}
	if srv != nil {
		t.Fatal("a server must not be returned alongside the error")
	}
	if !strings.Contains(err.Error(), "startup validation failed") {
		t.Fatalf("err = %v, want it to name the failed startup validation", err)
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("err = %v, want it to name the plugin", err)
	}
	if p.tools {
		t.Fatal("RegisterMCP ran for a plugin that failed validation")
	}
}

// validatingPlugin records the order validators ran in.
type validatingPlugin struct {
	name     string
	requires []string
	seen     *[]string
}

func (p validatingPlugin) Name() string { return p.name }
func (p validatingPlugin) Describe() plugin.Info {
	return plugin.Info{Title: p.name, Requires: p.requires}
}
func (p validatingPlugin) Models() []any                     { return nil }
func (p validatingPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (p validatingPlugin) Validate(_ context.Context, _ plugin.Host) error {
	*p.seen = append(*p.seen, p.name)
	return nil
}

// TestNewServerInstanceValidatesInDependencyOrder pins that the CLI validates
// in Requires order, the same order the HTTP boot path uses. A plugin whose
// Validate reads state a required plugin set up in its own Validate would
// otherwise pass or fail depending on registration order — and on which
// surface it was started.
func TestNewServerInstanceValidatesInDependencyOrder(t *testing.T) {
	var seen []string
	consumer := validatingPlugin{name: "consumer", requires: []string{"provider"}, seen: &seen}
	provider := validatingPlugin{name: "provider", seen: &seen}

	srv, err := NewServerInstance(context.Background(), nil, 1, []plugin.Plugin{consumer, provider}, nil)
	if err != nil {
		t.Fatalf("NewServerInstance: %v", err)
	}
	if srv == nil {
		t.Fatal("expected a server")
	}
	if got := strings.Join(seen, ","); got != "provider,consumer" {
		t.Fatalf("Validate order = %q, want %q", got, "provider,consumer")
	}
}

// TestNewServerInstanceRefusesDependencyCycle pins that the CLI rejects a
// plugin graph the HTTP boot path rejects, instead of mounting it in slice
// order and starting anyway.
func TestNewServerInstanceRefusesDependencyCycle(t *testing.T) {
	var seen []string
	a := validatingPlugin{name: "a", requires: []string{"b"}, seen: &seen}
	b := validatingPlugin{name: "b", requires: []string{"a"}, seen: &seen}

	srv, err := NewServerInstance(context.Background(), nil, 1, []plugin.Plugin{a, b}, nil)
	if err == nil {
		t.Fatal("expected a dependency cycle to abort the MCP server build")
	}
	if srv != nil {
		t.Fatal("a server must not be returned alongside the error")
	}
	if len(seen) != 0 {
		t.Fatalf("no validator may run on a graph that cannot be ordered, ran %v", seen)
	}
}
