package ontology

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
)

var ErrFrozen = errors.New("ontology: declarations are frozen; register before the plugin boots")

var ErrNoSpecs = errors.New("ontology: Register was called with no entity")

var ErrNoOwner = errors.New("ontology: Register needs the name of the plugin that declares the entity")

var OwnerPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func validateOwner(owner string) error {
	if owner == "" {
		return ErrNoOwner
	}
	if !OwnerPattern.MatchString(owner) {
		return fmt.Errorf("ontology: owner %q is not a plugin name (lowercase letters, digits, '-' and '_')", owner)
	}
	return nil
}

type Registry struct {
	mu     sync.Mutex
	specs  []Spec
	frozen bool
}

func NewRegistry() *Registry { return &Registry{} }

var defaultRegistry = NewRegistry()

func Default() *Registry { return defaultRegistry }

func MustRegister(owner string, specs ...Spec) { Default().MustRegister(owner, specs...) }

func (r *Registry) Register(owner string, specs ...Spec) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frozen {
		return ErrFrozen
	}
	if len(specs) == 0 {
		return ErrNoSpecs
	}
	if err := validateOwner(owner); err != nil {
		return err
	}

	seen := make(map[string]bool, len(r.specs)+len(specs))
	for _, s := range r.specs {
		seen[s.Slug] = true
	}
	owned := make([]Spec, len(specs))
	for i, s := range specs {
		s.Owner = owner
		if err := Validate(s); err != nil {
			return fmt.Errorf("ontology: registered entity %d: %w", i, err)
		}
		if seen[s.Slug] {
			return fmt.Errorf("ontology: entity %q is registered twice", s.Slug)
		}
		seen[s.Slug] = true
		owned[i] = s
	}

	r.specs = append(r.specs, owned...)
	return nil
}

func (r *Registry) MustRegister(owner string, specs ...Spec) {
	if err := r.Register(owner, specs...); err != nil {
		panic(err.Error())
	}
}

func (r *Registry) Declared() []Spec {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frozen = true
	out := make([]Spec, len(r.specs))
	copy(out, r.specs)
	return out
}
