package plugin

import (
	"context"
	"fmt"
)

// ValidateAll runs every plugin's Validator, in dependency order, against host.
//
// Three composition roots boot plugins — the HTTP server, the MCP server the app
// runs, and the stdio CLI — and each decided on its own whether to validate at
// all. The CLI did not, so a plugin that refuses to start without encryption
// registered its tools and failed on first use instead. One implementation means
// the three surfaces cannot drift again.
//
// host must be the host the plugins will actually run against. A no-op host
// handed to a validator that needs a real runtime turns a working server into a
// refused one; a real host handed to a mount that has none hides the refusal
// until the first call.
func ValidateAll(ctx context.Context, plugins []Plugin, host Host) error {
	if err := validateComposition(plugins); err != nil {
		return err
	}
	ordered, err := DependencyOrder(plugins)
	if err != nil {
		return err
	}
	for _, p := range ordered {
		v, ok := p.(Validator)
		if !ok {
			continue
		}
		if err := v.Validate(ctx, host); err != nil {
			return fmt.Errorf("plugin %q startup validation failed: %w", p.Name(), err)
		}
	}
	return nil
}

// validateComposition refuses a plugin set that cannot be judged: two plugins
// sharing a name, or a requirement that is not mounted.
//
// DependencyOrder treats a missing requirement as a leaf, because the HTTP root's
// preflight refuses that case before ordering ever runs. The other two roots have
// no preflight, so the shared entry point refuses it for all of them rather than
// leaving the division of labour to whoever calls this next.
func validateComposition(plugins []Plugin) error {
	mounted := make(map[string]bool, len(plugins))
	for _, p := range plugins {
		name := p.Name()
		if mounted[name] {
			return fmt.Errorf("plugin %q is mounted twice", name)
		}
		mounted[name] = true
	}
	for _, p := range plugins {
		for _, req := range Describe(p).Requires {
			if !mounted[req] {
				return fmt.Errorf("plugin %q requires %q, which is not mounted", p.Name(), req)
			}
		}
	}
	return nil
}
