package mcp

import (
	"testing"

	"github.com/octarq-org/octarq/server/plugins/builtin"
)

// TestNewServerInstanceMountsBuiltins pins the stdio composition contract: the
// built-in plugin set mounts with no HTTP surface, because this path wants MCP
// tools, not HTTP routes. A plugin that registers huma operations
// unconditionally panics there — which crashed `octarq-pro mcp` (and every
// MCP-SSE connection) at startup. A panic here is that crash, caught before
// release.
func TestNewServerInstanceMountsBuiltins(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("mounting the built-in plugins panicked with no HTTP surface: %v\n\n"+
				"The MCP composition path mounts plugins with a nil huma API, a nil "+
				"mux and a nil Guard. Skip HTTP registration when they are absent.", r)
		}
	}()
	if srv := NewServerInstance(nil, 1, builtin.Default(), nil); srv == nil {
		t.Fatal("NewServerInstance returned nil")
	}
}
