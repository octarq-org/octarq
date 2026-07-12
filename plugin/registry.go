package plugin

import (
	"fmt"
	"sync"
)

// Registry is the inter-plugin service registry backing Context.Provide and
// Context.Lookup. The app creates exactly one per process and shares it across
// every plugin's Context, so a service one plugin Provides is visible to all
// others. Plugins never construct a Registry themselves — they only see the
// Provide/Lookup funcs on Context; the type is exported for the app (and
// tests) to wire.
//
// Concurrency: Mount runs plugin-by-plugin on a single goroutine, so Provide
// is not raced in practice, but the registry locks anyway; Lookup takes a
// read lock and is safe from any goroutine (Start, request handlers).
type Registry struct {
	mu   sync.RWMutex
	svcs map[string]any
	errs []error
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{svcs: make(map[string]any)}
}

// Provide registers svc under name ("<pluginName>.<service>" by convention).
// A duplicate name is not overwritten; it is recorded as an error that Err
// returns, so the app can refuse startup after the mount phase instead of
// two plugins silently fighting over a name.
func (r *Registry) Provide(name string, svc any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.svcs[name]; dup {
		r.errs = append(r.errs, fmt.Errorf("plugin service %q provided twice", name))
		return
	}
	r.svcs[name] = svc
}

// Lookup returns the service registered under name, and whether it exists.
func (r *Registry) Lookup(name string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	svc, ok := r.svcs[name]
	return svc, ok
}

// Err reports any duplicate-Provide collisions recorded so far (nil if none).
// The app checks it once, after every plugin has mounted, and refuses to
// serve on a non-nil result — consistent with how other startup misconfig
// (e.g. table collisions) is reported as a returned error, not a panic.
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

// LookupAs resolves the named service and type-asserts it to T, returning
// (zero, false) when the service is absent or has a different type. It is the
// canonical consumer shape, paired with lazy (Start-time or per-request)
// resolution:
//
//	// Provider side, in Mount — hello owns the Greeter interface:
//	type Greeter interface{ Greet(who string) string }
//
//	func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
//		ctx.Provide("hello.greeter", Greeter(greeter{}))
//	}
//
//	// Consumer side — keep the Context from Mount, resolve in Start, which
//	// the app runs only after ALL plugins have mounted:
//	func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) { p.ctx = ctx }
//
//	func (p *Plugin) Start(ctx context.Context) {
//		g, ok := plugin.LookupAs[hello.Greeter](p.ctx, "hello.greeter")
//		if !ok {
//			return // provider not in this build — degrade gracefully
//		}
//		_ = g.Greet("world")
//	}
//
//	// Compile-time assertions (see the package doc convention):
//	var (
//		_ plugin.Plugin  = (*Plugin)(nil)
//		_ plugin.Starter = (*Plugin)(nil)
//	)
func LookupAs[T any](ctx *Context, name string) (T, bool) {
	var zero T
	if ctx == nil || ctx.Lookup == nil {
		return zero, false
	}
	svc, ok := ctx.Lookup(name)
	if !ok {
		return zero, false
	}
	t, ok := svc.(T)
	if !ok {
		return zero, false
	}
	return t, true
}
