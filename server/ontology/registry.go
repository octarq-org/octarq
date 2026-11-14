package ontology

import (
	"errors"
	"fmt"
	"sync"
)

// ErrFrozen is returned by Register once the declaration set has been read.
//
// A registration that arrives after boot would reach nothing: the tables, the
// routes and the tools are all built from the set that was read, so the entity
// would sit in the registry and exist nowhere else. Refusing it is the only
// answer that does not silently drop a declaration.
var ErrFrozen = errors.New("ontology: declarations are frozen; register before the plugin boots")

// ErrNoSpecs is returned by Register with nothing to register. A module that
// called it with an empty slice meant to declare something.
var ErrNoSpecs = errors.New("ontology: Register was called with no entity")

// Registry collects the entity declarations a module makes in Go.
//
// The declaration file is the operator's source of truth; this is the module
// author's. The two sides meet here because they cannot meet anywhere else: a
// composition root receives []plugin.Plugin from builtin.Default() and mounts
// the values it is given, so no module ever holds the plugin instance to hand it
// a declaration. A module therefore registers at init and the plugin reads the
// whole set once at boot.
//
// Registration is not applied immediately. Boot validates the file's entities
// and the registered ones together, so a slug declared in both is refused before
// any statement runs.
type Registry struct {
	mu     sync.Mutex
	specs  []Spec
	frozen bool
}

// NewRegistry returns an empty registry. Tests and a composition root that wants
// its declarations kept apart from the process-wide set use it; everything else
// uses Default.
func NewRegistry() *Registry { return &Registry{} }

var defaultRegistry = NewRegistry()

// Default returns the registry the plugin built by New reads. It is the
// process-wide set, so a module registers once, at init, and every plugin
// instance sees it.
func Default() *Registry { return defaultRegistry }

// Register appends declarations and validates each one immediately, so a module
// that registers at init fails there rather than at boot.
//
// Duplicates are checked within the registry only. A slug that also appears in
// the declaration file is caught at boot, where both sets are known.
func (r *Registry) Register(specs ...Spec) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frozen {
		return ErrFrozen
	}
	if len(specs) == 0 {
		return ErrNoSpecs
	}

	seen := make(map[string]bool, len(r.specs)+len(specs))
	for _, s := range r.specs {
		seen[s.Slug] = true
	}
	for i, s := range specs {
		if err := Validate(s); err != nil {
			return fmt.Errorf("ontology: registered entity %d: %w", i, err)
		}
		if seen[s.Slug] {
			return fmt.Errorf("ontology: entity %q is registered twice", s.Slug)
		}
		seen[s.Slug] = true
	}

	r.specs = append(r.specs, specs...)
	return nil
}

// MustRegister is Register for init, where there is no caller to return to. A
// declaration that cannot be recorded is a programming error in the module that
// wrote it, and the panic names it.
func (r *Registry) MustRegister(specs ...Spec) {
	if err := r.Register(specs...); err != nil {
		panic(err.Error())
	}
}

// Declared freezes the registry and returns what it holds, in registration
// order. Freezing is what makes "register at init, read at boot" safe: a second
// reader gets the same set, and a late writer gets ErrFrozen.
func (r *Registry) Declared() []Spec {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frozen = true
	out := make([]Spec, len(r.specs))
	copy(out, r.specs)
	return out
}
