package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

// cryptoMCPPlugin refuses to start without a crypto vault, the shape
// octarq-plugins/twofa uses. It is the reason the MCP path must validate
// against the host it mounts: the stdio transport re-mounts plugins with a
// no-op host whose Crypto() is nil, so a validator run there would refuse a
// plugin the server itself runs happily.
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

// TestRunMCPValidatesAgainstItsOwnHost pins both halves: RunMCP validates, and
// it validates against the host the plugins read from rather than the no-op
// host the stdio re-mount builds.
func TestRunMCPValidatesAgainstItsOwnHost(t *testing.T) {
	a := bootApp(t)
	p := &cryptoMCPPlugin{}
	a.Use(p)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = a.RunMCP(ctx)

	if !p.validated {
		t.Fatal("RunMCP did not validate the plugin")
	}
	if !p.registered {
		t.Fatal("plugin was refused: RunMCP validated against a host with no crypto vault")
	}
}

// TestRunMCPStopsBeforeRegisteringToolsOnRefusal is the fail-closed half: a
// plugin that refuses to start must not have its tools exposed to the agent.
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
