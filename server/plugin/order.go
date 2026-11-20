package plugin

import (
	"fmt"
	"strings"
)

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
