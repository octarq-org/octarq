package ontology

import (
	"errors"
	"fmt"
	"regexp"
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

// ErrNoOwner is returned by Register without a plugin name. An entity with no
// owner would be reachable in every workspace forever, which is the state this
// argument exists to make impossible.
var ErrNoOwner = errors.New("ontology: Register needs the name of the plugin that declares the entity")

// OwnerPattern is the shape an owner may take. It is a plugin name, so it is
// the same lowercase token the plugin contract already requires; a name that
// does not match cannot belong to a composed plugin.
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

// MustRegister declares entities in the process-wide registry. It is the form a
// module calls from init, where there is no caller to return an error to: a
// declaration the registry refuses is a programming error in the module that
// wrote it, so it stops the process instead of leaving half a schema behind.
//
// owner is the plugin that declares them, by Name(). The entity follows that
// plugin's per-workspace feature switch: turning the plugin off hides the page,
// its menu entry and its agent tools. An operator-declared entity (the
// declaration file) has no owner and no switch.
func MustRegister(owner string, specs ...Spec) { Default().MustRegister(owner, specs...) }

// Register appends declarations and validates each one immediately, so a module
// that registers at init fails there rather than at boot.
//
// Duplicates are checked within the registry only. A slug that also appears in
// the declaration file is caught at boot, where both sets are known.
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
		// The owner is the registry's to stamp, not the declaration's: the file
		// cannot name one, and a module that could would be able to claim an
		// entity another plugin's switch governs.
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

// MustRegister is Register for init, where there is no caller to return to. A
// declaration that cannot be recorded is a programming error in the module that
// wrote it, and the panic names it.
func (r *Registry) MustRegister(owner string, specs ...Spec) {
	if err := r.Register(owner, specs...); err != nil {
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
