package plugin

import (
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu              sync.RWMutex
	svcs            map[string]any
	healthProviders []HealthProvider
	errs            []error
}

func NewRegistry() *Registry {
	return &Registry{svcs: make(map[string]any)}
}

func (r *Registry) Provide(name string, svc any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == ServiceHealthProvider {
		if hp, ok := svc.(HealthProvider); ok {
			for _, existing := range r.healthProviders {
				if existing.Name() == hp.Name() {
					r.errs = append(r.errs, fmt.Errorf("plugin service %q provided twice for provider %q", name, hp.Name()))
					return
				}
			}
			r.healthProviders = append(r.healthProviders, hp)
			r.svcs[HealthProviderServiceName(hp.Name())] = hp
			if _, exists := r.svcs[ServiceHealthProvider]; !exists {
				r.svcs[ServiceHealthProvider] = hp
			}
			return
		}
		if hps, ok := svc.([]HealthProvider); ok {
			for _, hp := range hps {
				for _, existing := range r.healthProviders {
					if existing.Name() == hp.Name() {
						r.errs = append(r.errs, fmt.Errorf("plugin service %q provided twice for provider %q", name, hp.Name()))
						return
					}
				}
				r.healthProviders = append(r.healthProviders, hp)
				r.svcs[HealthProviderServiceName(hp.Name())] = hp
			}
			if len(hps) > 0 {
				if _, exists := r.svcs[ServiceHealthProvider]; !exists {
					r.svcs[ServiceHealthProvider] = hps[0]
				}
			}
			return
		}
	}

	if strings.HasPrefix(name, "health.provider.") {
		if hp, ok := svc.(HealthProvider); ok {
			for _, existing := range r.healthProviders {
				if existing.Name() == hp.Name() {
					r.errs = append(r.errs, fmt.Errorf("plugin service %q provided twice", name))
					return
				}
			}
			r.healthProviders = append(r.healthProviders, hp)
			r.svcs[name] = hp
			if _, exists := r.svcs[ServiceHealthProvider]; !exists {
				r.svcs[ServiceHealthProvider] = hp
			}
			return
		}
	}

	if _, dup := r.svcs[name]; dup {
		r.errs = append(r.errs, fmt.Errorf("plugin service %q provided twice", name))
		return
	}
	r.svcs[name] = svc
}

func (r *Registry) Lookup(name string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == ServiceHealthProvidersAll {
		res := make([]HealthProvider, len(r.healthProviders))
		copy(res, r.healthProviders)
		return res, len(res) > 0
	}
	svc, ok := r.svcs[name]
	return svc, ok
}

func (r *Registry) HealthProviders() []HealthProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]HealthProvider, len(r.healthProviders))
	copy(res, r.healthProviders)
	return res
}

func (r *Registry) Err() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.errs) == 0 {
		return nil
	}
	if len(r.errs) == 1 {
		return r.errs[0]
	}
	return fmt.Errorf("%d plugin service collisions, first: %w", len(r.errs), r.errs[0])
}

func LookupAs[T any](ctx *Context, name string) (T, bool) {
	if ctx == nil || ctx.Lookup == nil {
		var zero T
		return zero, false
	}
	return LookupServiceAs[T](ctx.Lookup, name)
}

func LookupServiceAs[T any](lookup func(name string) (any, bool), name string) (T, bool) {
	var zero T
	if lookup == nil {
		return zero, false
	}
	svc, ok := lookup(name)
	if !ok {
		return zero, false
	}
	t, ok := svc.(T)
	if !ok {
		return zero, false
	}
	return t, true
}
