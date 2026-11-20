package harness

import (
	"context"
	"fmt"
)

type SpecGuard struct {
	allowed map[string]bool
}

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

func (g *SpecGuard) Allow(_ context.Context, _ uint, tool, _ string) error {
	if !g.allowed[tool] {
		return fmt.Errorf("tool %q is not in the published harness allowlist", tool)
	}
	return nil
}

type chainGuard struct {
	guards []Guard
}

func (c *chainGuard) Allow(ctx context.Context, orgID uint, tool, argsJSON string) error {
	for _, g := range c.guards {
		if err := g.Allow(ctx, orgID, tool, argsJSON); err != nil {
			return err
		}
	}
	return nil
}

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
