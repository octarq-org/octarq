package app

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// namedPlugin is a bare plugin that only carries a name and a Requires list.
type namedPlugin struct {
	name     string
	requires []string
}

func (p namedPlugin) Name() string { return p.name }
func (p namedPlugin) Describe() plugin.Info {
	return plugin.Info{Title: p.name, Requires: p.requires}
}
func (p namedPlugin) Models() []any                     { return nil }
func (p namedPlugin) Mount(plugin.Mux, *plugin.Context) {}

func names(plugins []plugin.Plugin) []string {
	out := make([]string, len(plugins))
	for i, p := range plugins {
		out[i] = p.Name()
	}
	return out
}

func TestDependencyOrderPutsRequirementsFirst(t *testing.T) {
	// Registered in exactly the wrong order: mail before links before dns.
	plugins := []plugin.Plugin{
		namedPlugin{name: "mail", requires: []string{"dns", "links"}},
		namedPlugin{name: "links", requires: []string{"dns"}},
		namedPlugin{name: "dns"},
	}
	got, err := dependencyOrder(plugins)
	if err != nil {
		t.Fatalf("dependencyOrder: %v", err)
	}
	want := []string{"dns", "links", "mail"}
	if strings.Join(names(got), ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", names(got), want)
	}
}

// TestDependencyOrderIsStableForIndependentPlugins pins that the ordering does
// not shuffle a composition that has no dependencies: registration order is
// the operator's intent and must survive.
func TestDependencyOrderIsStableForIndependentPlugins(t *testing.T) {
	plugins := []plugin.Plugin{
		namedPlugin{name: "alpha"},
		namedPlugin{name: "beta"},
		namedPlugin{name: "gamma"},
	}
	got, err := dependencyOrder(plugins)
	if err != nil {
		t.Fatalf("dependencyOrder: %v", err)
	}
	if strings.Join(names(got), ",") != "alpha,beta,gamma" {
		t.Fatalf("order = %v, want registration order preserved", names(got))
	}
}

func TestDependencyOrderHandlesDiamond(t *testing.T) {
	// top requires left and right; both require base.
	plugins := []plugin.Plugin{
		namedPlugin{name: "top", requires: []string{"left", "right"}},
		namedPlugin{name: "right", requires: []string{"base"}},
		namedPlugin{name: "left", requires: []string{"base"}},
		namedPlugin{name: "base"},
	}
	got, err := dependencyOrder(plugins)
	if err != nil {
		t.Fatalf("dependencyOrder: %v", err)
	}
	pos := map[string]int{}
	for i, p := range got {
		pos[p.Name()] = i
	}
	if len(got) != 4 {
		t.Fatalf("order = %v, want all four plugins exactly once", names(got))
	}
	for _, edge := range [][2]string{{"base", "left"}, {"base", "right"}, {"left", "top"}, {"right", "top"}} {
		if pos[edge[0]] >= pos[edge[1]] {
			t.Errorf("%s must come before %s, order = %v", edge[0], edge[1], names(got))
		}
	}
}

// TestDependencyOrderRejectsCycle pins that a cycle fails startup by name
// instead of overflowing the stack. A self-requirement is the one-character
// version of the same mistake.
func TestDependencyOrderRejectsCycle(t *testing.T) {
	cases := []struct {
		name    string
		plugins []plugin.Plugin
		wantSub string
	}{
		{
			name: "self",
			plugins: []plugin.Plugin{
				namedPlugin{name: "solo", requires: []string{"solo"}},
			},
			wantSub: "solo -> solo",
		},
		{
			name: "two",
			plugins: []plugin.Plugin{
				namedPlugin{name: "a", requires: []string{"b"}},
				namedPlugin{name: "b", requires: []string{"a"}},
			},
			wantSub: "a -> b -> a",
		},
		{
			name: "three",
			plugins: []plugin.Plugin{
				namedPlugin{name: "a", requires: []string{"c"}},
				namedPlugin{name: "b", requires: []string{"a"}},
				namedPlugin{name: "c", requires: []string{"b"}},
			},
			wantSub: "dependency cycle",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dependencyOrder(tc.plugins)
			if err == nil {
				t.Fatalf("dependencyOrder returned %v, want a cycle error", names(got))
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantSub)
			}
			if !strings.Contains(err.Error(), "plugin.Info.Requires") {
				t.Errorf("error does not say where to look: %v", err)
			}
		})
	}
}

// TestDependencyOrderIgnoresMissingRequirement pins the division of labour:
// preflightDependencies already refuses a composition that is missing a
// required plugin, and it does so with a message naming both plugins. Failing
// here as well would replace that message with a worse one.
func TestDependencyOrderIgnoresMissingRequirement(t *testing.T) {
	plugins := []plugin.Plugin{
		namedPlugin{name: "needy", requires: []string{"absent"}},
		namedPlugin{name: "other"},
	}
	got, err := dependencyOrder(plugins)
	if err != nil {
		t.Fatalf("dependencyOrder: %v", err)
	}
	if strings.Join(names(got), ",") != "needy,other" {
		t.Fatalf("order = %v, want registration order preserved", names(got))
	}
}

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
