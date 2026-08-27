package tenantsql

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/plugin"
)

var validViewNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// Registry manages registered tenant views in memory.
// It is safe for concurrent use by multiple goroutines.
type Registry struct {
	mu    sync.RWMutex
	views map[string]plugin.TenantView
}

// NewRegistry creates a new empty tenant view registry.
func NewRegistry() *Registry {
	return &Registry{
		views: make(map[string]plugin.TenantView),
	}
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

// DefaultRegistry returns the package-level default singleton Registry.
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
	})
	return defaultRegistry
}

// Register adds a new TenantView to the registry.
// It validates that the view has a non-empty name starting with DefaultViewPrefix ("tenant_"),
// a non-nil Definition function, and has not already been registered.
func (r *Registry) Register(view plugin.TenantView) error {
	name := strings.TrimSpace(view.Name)
	if name == "" {
		return errors.New("view name cannot be empty")
	}
	if !validViewNameRegex.MatchString(name) {
		return fmt.Errorf("view name %q contains invalid characters (only alphanumeric and underscores are allowed)", view.Name)
	}
	if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(DefaultViewPrefix)) {
		return fmt.Errorf("view name %q must start with required prefix %q", view.Name, DefaultViewPrefix)
	}
	if view.Definition == nil {
		return fmt.Errorf("view definition func for %q cannot be nil", view.Name)
	}

	key := strings.ToLower(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.views[key]; exists {
		return fmt.Errorf("view %q already registered", view.Name)
	}

	r.views[key] = view
	return nil
}

// Lookup finds a registered view by name (case-insensitive).
func (r *Registry) Lookup(name string) (plugin.TenantView, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	r.mu.RLock()
	defer r.mu.RUnlock()

	view, ok := r.views[key]
	return view, ok
}

// List returns all registered views, sorted alphabetically by Name.
func (r *Registry) List() []plugin.TenantView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	views := make([]plugin.TenantView, 0, len(r.views))
	for _, v := range r.views {
		views = append(views, v)
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].Name < views[j].Name
	})

	return views
}

// Reset clears all registered views in the registry (primarily for test teardown).
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.views = make(map[string]plugin.TenantView)
}
