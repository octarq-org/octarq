package plugin

import (
	"context"
	"fmt"
)

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
