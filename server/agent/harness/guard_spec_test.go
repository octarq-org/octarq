package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
)

// TestSpecGuardRefusesAToolOutsideTheAllowlist pins the control the offline
// lab has always enforced: a published allowlist is a control, not a hint.
func TestSpecGuardRefusesAToolOutsideTheAllowlist(t *testing.T) {
	spec := harness.HarnessSpec{
		Version:   harness.HarnessSpecVersion,
		Name:      "narrow",
		ToolAllow: []string{"read_things"},
	}
	guard := harness.NewSpecGuard(spec)

	if err := guard.Allow(context.Background(), 1, "read_things"); err != nil {
		t.Fatalf("a tool inside the allowlist was refused: %v", err)
	}
	err := guard.Allow(context.Background(), 1, "read_more")
	if err == nil {
		t.Fatal("a tool outside the allowlist was permitted")
	}
	if !strings.Contains(err.Error(), "allowlist") {
		t.Errorf("the refusal does not name the allowlist: %v", err)
	}
}

// TestSpecGuardWithoutAnAllowlistPermitsEverything: an empty allowlist means
// "no restriction". A spec narrows the tool surface; it never widens it.
func TestSpecGuardWithoutAnAllowlistPermitsEverything(t *testing.T) {
	guard := harness.NewSpecGuard(harness.DefaultHarnessSpec())
	if err := guard.Allow(context.Background(), 1, "anything"); err != nil {
		t.Fatalf("an empty allowlist refused a tool: %v", err)
	}
}

// guardFunc adapts a function to the Guard interface.
type guardFunc func(context.Context, uint, string) error

// Allow implements Guard.
func (f guardFunc) Allow(ctx context.Context, orgID uint, tool string) error {
	return f(ctx, orgID, tool)
}

// TestComposeGuardsRequiresEveryGuardToPermit: a channel composes its own
// rules with the workspace's published spec, and the strictest wins.
func TestComposeGuardsRequiresEveryGuardToPermit(t *testing.T) {
	allow := guardFunc(func(context.Context, uint, string) error { return nil })
	deny := guardFunc(func(context.Context, uint, string) error { return context.Canceled })

	guard := harness.ComposeGuards(allow, deny)
	if err := guard.Allow(context.Background(), 1, "tool"); err == nil {
		t.Fatal("the chain permitted a tool one of its guards refused")
	}

	// A chain of one behaves exactly like that guard, and a chain of none
	// permits everything.
	single := harness.ComposeGuards(allow)
	if err := single.Allow(context.Background(), 1, "tool"); err != nil {
		t.Fatalf("a chain of one refused a tool: %v", err)
	}
	empty := harness.ComposeGuards()
	if err := empty.Allow(context.Background(), 1, "tool"); err != nil {
		t.Fatalf("an empty chain refused a tool: %v", err)
	}
}
