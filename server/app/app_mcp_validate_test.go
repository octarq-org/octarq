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
// against the host the stdio mount builds: that mount hands every plugin a
// no-op host whose Crypto() is nil, so a validator run there refuses a plugin
// whose tools could never encrypt.
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

// TestRunMCPValidatesAgainstTheHostTheMountBuilds pins the host choice. The
// stdio mount re-mounts every plugin through buildServerInstance with a no-op
// host, and a plugin caches the host it is handed at Mount, so the no-op host is
// the one its tools read from. Validating the host RunMCP built would pass this
// plugin and expose tools that fail on every call.
func TestRunMCPValidatesAgainstTheHostTheMountBuilds(t *testing.T) {
	a := bootApp(t)
	p := &cryptoMCPPlugin{}
	a.Use(p)

	err := a.RunMCP(context.Background())
	if !p.validated {
		t.Fatal("RunMCP did not validate the plugin")
	}
	if err == nil || !strings.Contains(err.Error(), "encryption unavailable") {
		t.Fatalf("RunMCP = %v, want the plugin refused: the stdio mount gives it no vault", err)
	}
	if p.registered {
		t.Fatal("tools were registered for a plugin the stdio mount cannot serve")
	}
}

// TestRunMCPStopsBeforeRegisteringToolsOnRefusal is the fail-closed half: a
// plugin that refuses to start must not have its tools exposed to the agent,
// and the refusal must name the plugin so an operator knows what to fix.
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
