package plugin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// validatingPlugin records the order validators ran in and can refuse.
type validatingPlugin struct {
	namedPlugin
	order   *[]string
	refuse  error
	hostGot *plugin.Host
}

func (p validatingPlugin) Validate(_ context.Context, host plugin.Host) error {
	*p.order = append(*p.order, p.name)
	if p.hostGot != nil {
		*p.hostGot = host
	}
	return p.refuse
}

func TestValidateAllRunsInDependencyOrder(t *testing.T) {
	var order []string
	// base is registered last but required by dependent, so registration order
	// would run dependent first and hand it a registry that is still empty.
	plugins := []plugin.Plugin{
		validatingPlugin{namedPlugin: namedPlugin{name: "dependent", requires: []string{"base"}}, order: &order},
		validatingPlugin{namedPlugin: namedPlugin{name: "base"}, order: &order},
		namedPlugin{name: "plain"},
	}

	if err := plugin.ValidateAll(context.Background(), plugins, nil); err != nil {
		t.Fatalf("ValidateAll: %v", err)
	}
	want := []string{"base", "dependent"}
	if len(order) != len(want) {
		t.Fatalf("validation order = %v, want %v (non-validators must be skipped)", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("validation order = %v, want %v", order, want)
		}
	}
}

func TestValidateAllNamesThePluginThatRefused(t *testing.T) {
	var order []string
	refusal := errors.New("neither Host.Crypto nor a key is configured")
	err := plugin.ValidateAll(context.Background(), []plugin.Plugin{
		validatingPlugin{namedPlugin: namedPlugin{name: "base"}, order: &order},
		validatingPlugin{namedPlugin: namedPlugin{name: "needs-crypto"}, order: &order, refuse: refusal},
		validatingPlugin{namedPlugin: namedPlugin{name: "after"}, order: &order},
	}, nil)
	if !errors.Is(err, refusal) {
		t.Fatalf("err = %v, want the plugin's own error", err)
	}
	if !strings.Contains(err.Error(), `plugin "needs-crypto"`) {
		t.Fatalf("err = %v, want the refusing plugin named", err)
	}
	// A refusal must stop the boot, not collect every failure and continue.
	if len(order) != 2 {
		t.Fatalf("validation order = %v, want it to stop at the refusal", order)
	}
}

func TestValidateAllRefusesDependencyCycle(t *testing.T) {
	var order []string
	err := plugin.ValidateAll(context.Background(), []plugin.Plugin{
		validatingPlugin{namedPlugin: namedPlugin{name: "a", requires: []string{"b"}}, order: &order},
		validatingPlugin{namedPlugin: namedPlugin{name: "b", requires: []string{"a"}}, order: &order},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("err = %v, want a dependency cycle", err)
	}
	if len(order) != 0 {
		t.Fatalf("order = %v, want no validator to run before the cycle is detected", order)
	}
}

func TestValidateAllPassesTheGivenHost(t *testing.T) {
	var order []string
	var got plugin.Host
	host := plugin.EnsureHost(nil)
	if err := plugin.ValidateAll(context.Background(), []plugin.Plugin{
		validatingPlugin{namedPlugin: namedPlugin{name: "p"}, order: &order, hostGot: &got},
	}, host); err != nil {
		t.Fatalf("ValidateAll: %v", err)
	}
	if got != host {
		t.Fatalf("validator saw %#v, want the host the caller passes", got)
	}
}
