package mcp

import (
	"testing"

	"github.com/octarq-org/octarq/server/plugins/builtin"
)

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
