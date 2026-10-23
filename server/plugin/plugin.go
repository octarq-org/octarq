// Package plugin defines the public, importable contract that anyone implements to
// extend octarq without forking it.
//
// Octarq's own core features (links, mail, domains) and the Pro
// edition are built on this exact same seam — a community plugin can do
// everything an official plugin can do. The open-core binary depends only on
// this package and app; custom binaries and downstream distributions register plugins
// through it.
//
// AutoMigrate timing: a plugin contributes its GORM models via Models(). The
// app intentionally does NOT migrate at db-open time — it waits until every
// plugin has been registered, then runs AutoMigrate over the union of core and
// plugin models exactly once. A plugin therefore never has to (and must not)
// call AutoMigrate itself; doing so early would race the core schema.
//
// # Optional capabilities and compile-time assertions
//
// Beyond the required Plugin interface, a plugin opts into extra capabilities
// by implementing optional interfaces (Starter, MenuProvider, MCPProvider,
// OpenAPIContributor, Describer). The app detects them via runtime type
// assertion, which means a typo'd method name or a drifted signature does NOT
// fail the build — the capability is silently never invoked. To catch that at
// compile time, every plugin MUST pair each optional interface it implements
// with a compile-time assertion:
//
//	var (
//		_ plugin.Plugin       = (*Plugin)(nil)
//		_ plugin.Starter      = (*Plugin)(nil)
//		_ plugin.MenuProvider = (*Plugin)(nil)
//	)
//
// (use the value form `Plugin{}` if the plugin uses value receivers). See
// examples/plugin-hello for the canonical shape.
//
// # Inter-plugin services
//
// Two separately-compiled plugins interact through the service registry on
// Context — Provide and Lookup — never by importing each other. A provider
// registers a service value during Mount under a stable string name; the
// naming convention is "<pluginName>.<service>" (e.g. "billing.issuer",
// "hello.greeter"). The service value should be an interface defined in an
// importable package (the provider's own, or a shared contract package) so a
// consumer can retrieve it with the typed helper LookupAs.
//
// Lifecycle rules:
//
//   - Mount runs once per plugin, in registration (app.Use) order, on a
//     single goroutine. Provide must only be called during Mount.
//   - Lookup during Mount only sees services from plugins registered earlier.
//     Registration order is an app-wiring detail, so a cross-plugin consumer
//     must resolve lazily — in Start (the app launches Start goroutines only
//     after every plugin has mounted) or per-request — and degrade gracefully
//     when the name is absent (the provider may not be in this build).
//   - Providing the same name twice is a startup error: app.Run and
//     app.RunMCP refuse to serve.
//   - After the mount phase the registry is effectively read-only and safe
//     for concurrent Lookup from any goroutine.
//
// See Registry for the backing implementation and LookupAs for the canonical
// consumer shape.
//
// # Context evolution policy
//
// Context is a struct of exported fields, most of them funcs the host fills in.
// Plugins compile against it from a separate module, so what may change and how
// is part of the contract, not an implementation detail.
//
// Additive and safe — no plugin has to change:
//
//   - Adding a new field (func or data). Existing plugins keep compiling; they
//     simply never touch it. This is how the seam is meant to grow.
//   - Adding a new optional interface a Plugin may implement (see the section
//     above). Plugins that do not implement it are unaffected.
//   - Adding a new well-known service name and its named contract type in
//     services.go. Nothing resolves a name it does not know.
//
// Breaking — every plugin in every build must change together:
//
//   - Changing the signature of any of the ~40 exported func fields, including
//     "compatible-looking" edits: adding a parameter, widening a return to a
//     (T, error) pair, or renaming the field. A plugin in another module fails
//     to build, which is the intended outcome — this is the one class of change
//     the compiler still catches across the seam.
//   - Removing a field, or changing what a value MEANS while keeping its type
//     (e.g. making OrgRole return the raw membership instead of the effective
//     role). The second kind is worse: nothing fails to build and every gate
//     written against the old meaning quietly decides differently.
//
// Because of that second kind, a semantic change to a field is treated as a
// breaking change even when the signature is untouched: it needs a new field
// (or a new name) rather than a redefinition in place.
//
// Nil fields and older hosts. A plugin may be mounted by a host older than the
// field it wants — a downstream plugin pinned ahead of the core it runs against, or a
// third-party plugin built against a newer release. The host leaves such a
// field nil, so every field added after the initial contract is documented with
// the marker phrase "nil on hosts that predate it", and a plugin MUST nil-check
// those before calling and degrade gracefully rather than panic. Fields without
// that marker are wired by every supported host. When you add a field, add the
// marker to its doc comment in the same commit; it is the only signal a plugin
// author has.
//
// Support window: the mechanism is the marker phrase plus the nil check — a
// field never becomes non-optional by fiat, it becomes non-optional when every
// host still in support wires it, at which point the marker is dropped. The
// supported-host window is the current minor and the one before it.
package plugin

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Mux is the subset of *http.ServeMux a plugin uses to register routes. The app
// passes a wrapper (not the raw mux) so it can gate every plugin route behind a
// per-workspace "plugin enabled" check without the plugin having to opt in.
// *http.ServeMux satisfies this interface, so plugin bodies are unchanged.
type Mux interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// Plugin is a unit of functionality mounted onto the core app.
type Plugin interface {
	// Name is a short, stable identifier used in logs, the plugin registry, and
	// the per-workspace enable/disable setting (e.g. "ai", "infra", "billing").
	Name() string
	// Models returns the GORM models this plugin owns. They are collected and
	// migrated together with core models before any route is served.
	Models() []any
	// Mount registers the plugin's HTTP routes (typically under /api/...) on the
	// shared API mux. Use ctx.Guard to require a session. Every route registered
	// here is automatically gated: if the plugin is disabled for the caller's
	// workspace, the app answers 404 before the handler runs.
	Mount(mux Mux, ctx *Context)
}

// Starter is an optional interface a Plugin may implement. If present, the app
// calls Start in a goroutine after ALL plugins are mounted, passing the
// server's root context so the plugin can run background work (e.g. schedulers)
// and stop cleanly on shutdown. Because every Mount (and therefore every
// Provide) has completed by then, Start is the earliest safe point to Lookup
// services from other plugins regardless of registration order.
type Starter interface {
	Start(ctx context.Context)
}

// Validator is an optional interface a Plugin may implement to validate its
// runtime dependencies, configurations, or environment pre-conditions during server startup.
// The host invokes Validate synchronously after all plugins have completed Mount.
// If any plugin returns an error, the host refuses to start (Fail-Closed).
type Validator interface {
	Validate(ctx context.Context, host Host) error
}

// MCPProvider is an optional interface a Plugin may implement if it registers
// dynamic MCP tools. MCP tools MUST use plugin.OrgIDFromContext(ctx) to retrieve
// the caller's org ID, and MUST NOT read org fields cached on the plugin during Mount
// (under networked transport that field belongs to another caller).
type MCPProvider interface {
	// MCPTier names the license tier every tool from this plugin requires,
	// in the same vocabulary an endpoint's Tier uses ("" = free). It is
	// declared here, not judged here: the MCP composition consults the
	// injected TierGate and never calls RegisterMCP for a tier the instance
	// does not cover. A plugin with tools of mixed tiers must expose the
	// higher one through an EndpointSpec per tool instead.
	MCPTier() string
	RegisterMCP(srv *mcp.Server)
}

// Help documentation types, taxonomy categories, and frontmatter parsers
// (HelpCategory, HelpDoc, ParseHelpDoc, etc.) are defined in helpdoc.go.

// HelpProvider is implemented by plugins that ship in-app documentation.
type HelpProvider interface {
	HelpDocs() []HelpDoc
}

// OpenAPIContributor is an optional interface a Plugin may implement if it
// registers paths or schemas in the OpenAPI specification.
type OpenAPIContributor interface {
	OpenAPIPaths() map[string]any
	OpenAPISchemas() map[string]any
}
