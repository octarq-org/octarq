package app_test

import (
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/builtin"
)

// TestBuiltinDefaultSet checks the OSS Core infrastructure plugins in dependency order.
func TestBuiltinDefaultSet(t *testing.T) {
	got := builtin.Default()
	names := make([]string, len(got))
	for i, p := range got {
		names[i] = p.Name()
	}
	want := []string{"help", "tenantquery", "notify", "llmgateway"}
	if len(names) != len(want) {
		t.Fatalf("Default() = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("Default() order = %v, want %v", names, want)
		}
	}
	// Every entry must be EnabledByDefault or Core.
	for _, p := range got {
		info := plugin.Describe(p)
		if !info.EnabledByDefault && !info.Core {
			t.Errorf("%s should be EnabledByDefault or Core", p.Name())
		}
	}
}
