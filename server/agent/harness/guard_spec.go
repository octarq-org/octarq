package harness

import (
	"context"
	"fmt"
)

// SpecGuard enforces a published spec's tool allowlist at execution time.
//
// The allowlist is a control, not a hint. A workspace that publishes
// "list_links only" must not have its assistant call anything else, whatever
// the model decides to do — the prompt tells the model which tools exist, but
// a model is not a security boundary. The offline lab enforced this from the
// start; the running agent did not, so a candidate that won the benchmark as
// "restricted" was in fact unrestricted in production.
//
// The guard narrows, never widens: an empty allowlist means "no restriction",
// because a spec may narrow the tool surface but never widen it.
type SpecGuard struct {
	allowed map[string]bool
}

// NewSpecGuard constructs a Guard enforcing the spec's ToolAllow allowlist.
func NewSpecGuard(spec HarnessSpec) Guard {
	if len(spec.ToolAllow) == 0 {
		return NopGuard{}
	}
	m := make(map[string]bool, len(spec.ToolAllow))
	for _, name := range spec.ToolAllow {
		m[name] = true
	}
	return &SpecGuard{allowed: m}
}

// Allow implements Guard.
func (g *SpecGuard) Allow(_ context.Context, _ uint, tool string) error {
	if !g.allowed[tool] {
		return fmt.Errorf("tool %q is not in the published harness allowlist", tool)
	}
	return nil
}

// chainGuard permits a tool only when every guard in the chain permits it.
type chainGuard struct {
	guards []Guard
}

// Allow implements Guard. The first refusal wins, so the message names the
// rule that refused rather than the last one consulted.
func (c *chainGuard) Allow(ctx context.Context, orgID uint, tool string) error {
	for _, g := range c.guards {
		if err := g.Allow(ctx, orgID, tool); err != nil {
			return err
		}
	}
	return nil
}

// ComposeGuards combines guards into one that permits a tool only when every
// guard permits it.
//
// This is how a channel composes its own rules (read-only, approval) with
// the workspace's published spec: the spec is one voice among several, and
// the strictest wins. A nil guard and a NopGuard are dropped, so a chain of
// one behaves exactly like that guard.
func ComposeGuards(guards ...Guard) Guard {
	kept := make([]Guard, 0, len(guards))
	for _, g := range guards {
		if g == nil {
			continue
		}
		if _, isNop := g.(NopGuard); isNop {
			continue
		}
		kept = append(kept, g)
	}
	switch len(kept) {
	case 0:
		return NopGuard{}
	case 1:
		return kept[0]
	default:
		return &chainGuard{guards: kept}
	}
}
