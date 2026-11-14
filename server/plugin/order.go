package plugin

import (
	"fmt"
	"strings"
)

// DependencyOrder returns plugins ordered so that every plugin comes after the
// plugins it lists in Describe().Requires.
//
// Registration order used to be the only order, which made correctness depend
// on the composition root calling Use in the right sequence. That is invisible
// and survives review: a plugin whose Validate checks that a service it needs
// is registered passes only because the provider happened to mount first, and
// a downstream build that assembles the same plugins in another order fails at
// boot with a message about a missing service nobody removed.
//
// Requires already exists and is already validated for completeness
// (preflightDependencies), so the graph is there; this only reads it. A name in
// Requires that is not mounted is a leaf here — preflight owns that diagnosis
// and refuses startup before this runs.
//
// The order is deterministic: independent plugins keep their registration
// order, and a plugin's dependencies are visited in the order it lists them.
//
// Every caller that runs Validate must use this order, or the guarantee above
// holds on one surface and not the other. That is why it lives here rather than
// in the composition root that used to own it.
func DependencyOrder(plugins []Plugin) ([]Plugin, error) {
	byName := make(map[string]Plugin, len(plugins))
	for _, p := range plugins {
		byName[p.Name()] = p
	}

	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(plugins))
	ordered := make([]Plugin, 0, len(plugins))
	var path []string

	var visit func(p Plugin) error
	visit = func(p Plugin) error {
		name := p.Name()
		switch state[name] {
		case done:
			return nil
		case visiting:
			// Report the cycle from the point it closes, not from the root:
			// "a -> b -> c -> a" points at the edge the author has to remove.
			start := 0
			for i, n := range path {
				if n == name {
					start = i
					break
				}
			}
			cycle := append(append([]string{}, path[start:]...), name)
			return fmt.Errorf(
				"plugin dependency cycle: %s — a plugin cannot require itself, directly or through a chain (check plugin.Info.Requires)",
				strings.Join(cycle, " -> "),
			)
		}
		state[name] = visiting
		path = append(path, name)
		for _, req := range Describe(p).Requires {
			dep, ok := byName[req]
			if !ok {
				continue
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		path = path[:len(path)-1]
		state[name] = done
		ordered = append(ordered, p)
		return nil
	}

	for _, p := range plugins {
		if err := visit(p); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
