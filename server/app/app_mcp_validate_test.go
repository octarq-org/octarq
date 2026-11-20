package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

type cryptoMCPPlugin struct {
	refuse     error
	validated  bool
	registered bool
}

func (p *cryptoMCPPlugin) Name() string                      { return "crypto-mcp" }
func (p *cryptoMCPPlugin) Models() []any                     { return nil }
func (p *cryptoMCPPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (p *cryptoMCPPlugin) MCPTier() string                   { return "" }
func (p *cryptoMCPPlugin) RegisterMCP(*mcp.Server)           { p.registered = true }

func (p *cryptoMCPPlugin) Validate(_ context.Context, host plugin.Host) error {
	p.validated = true
	if p.refuse != nil {
		return p.refuse
	}
	if host == nil || host.Crypto() == nil {
		return errors.New("encryption unavailable")
	}
	return nil
}

func TestRunMCPValidatesAgainstTheHostTheMountBuilds(t *testing.T) {
	a := bootApp(t)
	p := &cryptoMCPPlugin{}
	a.Use(p)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.RunMCP(ctx)
	if !p.validated {
		t.Fatal("RunMCP did not validate the plugin")
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunMCP = %v, want nil or context.Canceled when stdio host provides CryptoVault", err)
	}
	if !p.registered {
		t.Fatal("crypto-dependent MCP plugin tools were not registered")
	}
}

func TestRunMCPStopsBeforeRegisteringToolsOnRefusal(t *testing.T) {
	a := bootApp(t)
	refusal := errors.New("twofa: encryption unavailable")
	p := &cryptoMCPPlugin{refuse: refusal}
	a.Use(p)

	err := a.RunMCP(context.Background())
	if !errors.Is(err, refusal) {
		t.Fatalf("RunMCP = %v, want the plugin's own refusal", err)
	}
	if !strings.Contains(err.Error(), `plugin "crypto-mcp"`) {
		t.Fatalf("RunMCP = %v, want the refusing plugin named", err)
	}
	if p.registered {
		t.Fatal("tools were registered after the plugin refused to start")
	}
}
