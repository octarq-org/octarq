package app

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// validateRecorder is a plugin whose Validate appends its name to a shared
// slice, so a test can assert on the order Validate ran in.
type validateRecorder struct {
	name     string
	requires []string
	seen     *[]string
}

func (p validateRecorder) Name() string { return p.name }
func (p validateRecorder) Describe() plugin.Info {
	return plugin.Info{Title: p.name, Requires: p.requires}
}
func (p validateRecorder) Models() []any                     { return nil }
func (p validateRecorder) Mount(plugin.Mux, *plugin.Context) {}
func (p validateRecorder) Validate(context.Context, plugin.Host) error {
	*p.seen = append(*p.seen, p.name)
	return nil
}

// TestRunValidatesInDependencyOrder is the behavioural half: Run must call
// Validate in the order Requires declares, not the order the composition root
// called Use. A plugin whose Validate reads state a required plugin set up in
// its own Validate used to pass or fail depending on registration order.
//
// The ordering itself is tested in plugin/order_test.go; this pins that the
// HTTP boot path uses it.
func TestRunValidatesInDependencyOrder(t *testing.T) {
	a := bootApp(t)
	var seen []string
	// consumer first, provider second — the order that used to run backwards.
	a.Use(validateRecorder{name: "consumer", requires: []string{"provider"}, seen: &seen})
	a.Use(validateRecorder{name: "provider", seen: &seen})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if got := strings.Join(seen, ","); got != "provider,consumer" {
		t.Fatalf("Validate order = %q, want %q", got, "provider,consumer")
	}
}
