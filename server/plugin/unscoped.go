package plugin

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnscopedInvalid   = errors.New("invalid unscoped route declaration")
	ErrUnscopedDuplicate = errors.New("unscoped route declared by two plugins")
)

type UnscopedRouteProvider interface {
	UnscopedPrefixes() []string
}

type UnscopedDeclaration struct {
	Plugin string
	Path   string
}

func UnscopedPrefixes(p Plugin) []string {
	if provider, ok := p.(UnscopedRouteProvider); ok {
		return provider.UnscopedPrefixes()
	}
	return nil
}

func ValidateUnscopedDeclaration(decl string) error {
	if decl == "" {
		return fmt.Errorf("%w: declaration is empty", ErrUnscopedInvalid)
	}
	if decl != strings.TrimSpace(decl) {
		return fmt.Errorf("%w: %q has surrounding whitespace", ErrUnscopedInvalid, decl)
	}
	for _, r := range decl {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return fmt.Errorf("%w: %q contains whitespace", ErrUnscopedInvalid, decl)
		}
	}
	if !strings.HasPrefix(decl, "/") {
		return fmt.Errorf("%w: %q must be an absolute path", ErrUnscopedInvalid, decl)
	}
	if strings.ContainsAny(decl, "{}*?") {
		return fmt.Errorf("%w: %q must not contain route patterns", ErrUnscopedInvalid, decl)
	}
	if decl == "/" || decl == "/api" || decl == "/api/" {
		return fmt.Errorf("%w: %q is too broad", ErrUnscopedInvalid, decl)
	}
	if strings.HasSuffix(decl, "/") {
		if strings.HasSuffix(decl, "//") {
			return fmt.Errorf("%w: %q has an empty segment", ErrUnscopedInvalid, decl)
		}
		return nil
	}
	if strings.Contains(decl, "//") {
		return fmt.Errorf("%w: %q has an empty segment", ErrUnscopedInvalid, decl)
	}
	return nil
}

func CollectUnscopedDeclarations(plugins []Plugin) ([]UnscopedDeclaration, error) {
	owners := make(map[string]string)
	var out []UnscopedDeclaration
	for _, p := range plugins {
		name := p.Name()
		for _, decl := range UnscopedPrefixes(p) {
			if err := ValidateUnscopedDeclaration(decl); err != nil {
				return nil, fmt.Errorf("plugin %q: %w", name, err)
			}
			if prev, ok := owners[decl]; ok {
				if prev == name {
					continue
				}
				return nil, fmt.Errorf("%w: %q is declared by %q and %q", ErrUnscopedDuplicate, decl, prev, name)
			}
			owners[decl] = name
			out = append(out, UnscopedDeclaration{Plugin: name, Path: decl})
		}
	}
	return out, nil
}
