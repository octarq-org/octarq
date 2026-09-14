---
title: Plugin Architecture
description: The architecture and design principles of Octarq's symmetric, no-fork plugin system.
sidebar:
  order: 1
  group:
    label: "Architecture"
---


Reference for how Octarq's symmetric plugin system is designed and composed at build time. For a step-by-step developer tutorial on creating a plugin, see [Writing a Plugin](/writing-a-plugin/).

## 1. Principle: symmetric, no-fork plugins

Every platform capability is structured as a **plugin** with two corresponding halves:

- **Backend** — a Go module implementing `plugin.Plugin`.
- **Frontend** — a React package implementing `UIPlugin` (from `@octarq/plugin-sdk`).

Both halves compose **at build time**: compile-time Go interface implementations + build-time frontend manifest injection. The resulting single binary embeds the frontend assets via Go's standard `go:embed`.

## 2. Backend contract (`server/plugin/plugin.go`)

```go
type Plugin interface {
    Name() string                 // stable identifier; matches UIPlugin.name
    Models() []any                // GORM models (migrated for the plugin)
    Mount(mux Mux, ctx *Context)  // register HTTP routes
}

// Optional interfaces (paired with compile-time assertions):
type MenuProvider interface { Menus() []MenuItem }
type Starter interface { Start(ctx context.Context) }   // executes after all plugins are mounted
type MCPProvider interface { Tools() []mcp.Tool }
```

- **Auto-gating**: The host registers each plugin through `app.Use(p)`. Plugin routes are automatically gated by `gatedMux` (`server/app/app.go`): if a feature is toggled off for a workspace, the server immediately returns **404** before the handler executes.
- **Strict isolation**: Plugins never import `internal/*`. All necessary capabilities are provided through `plugin.Context` (`DB`, `Guard`, `Encrypt`/`Decrypt`, `Audit`, `Notify`, `SendMail`, `OnEmail`, `DNS`, `GetWorkspaceSetting`/`SetWorkspaceSetting`).
- **Inter-plugin services**: A provider registers a service via `ctx.Provide("<plugin>.<service>", svc)` during `Mount`; consumers resolve dependencies lazily using `plugin.LookupAs[T]`.
- **Preflight migration**: All plugin database models are inspected and migrated on startup (`server/app/preflight.go`), preventing conflicting table definitions across plugins.

## 3. Frontend contract & build-time composition

The `@octarq/plugin-sdk` package defines the UI contract:

```ts
interface UIPlugin {
  name: string                 // matches Go Plugin.Name()
  routes: { path: string; Component: LazyPage; requiredRole?: string }[]
  menu?: PluginMenuItem[]
  widgets?: UIWidget[]
  areas?: UIArea[]
  i18n?: { en: Record<string, string>; zh: Record<string, string> }
}
```

- **Manifest-driven registration**: The active UI plugins are declared in `web/octarq.plugins.json`.
- **Vite virtual module**: At build time, `web/plugins-manifest.ts` generates the `#octarq-plugins` virtual module, importing each active plugin and registering it with the app shell.
- **Degradation boundaries**: Routes are wrapped in `PluginGate`. If a plugin chunk fails to load or a route is disabled, the shell renders a clean fallback state rather than crashing the interface.

## 4. Shared UI (`@octarq/plugin-sdk`)

The SDK provides a consistent design system built on top of modern web standards:

- Built with **Tailwind CSS** and **Base UI** primitives for accessible, robust components (dialogs, toggles, tooltips, dropdowns).
- Re-exports core layout components, icons, and styling utilities (`cn`).
- Decoupled from backend state: UI plugins communicate exclusively through typed API endpoints and standard fetch facades.

## 5. Data & Execution Boundaries

Octarq enforces clear execution boundaries and trust tiers across its runtime layers:

| Layer | Execution Boundary & Scope | Trust Level | Primary Responsibilities | Data & Network Access | Security Invariants |
|---|---|---|---|---|---|
| **Host Core Engine** | System host process (`app.App`) | **Root / Full Authority** | HTTP lifecycle, authentication sessions, CSRF validation, tenant resolution, route dispatching, database connection pool, graceful shutdown. | Direct database access (SQLite/Postgres), environment variables, server network sockets. | Immediate session revocation on role changes; centralized rate limiting; zero raw error leakage. |
| **Plugin Context (`plugin.Context`)** | In-process module sandbox facade | **High (Operator-Curated)** | Business feature logic, plugin route mounting (`Mount`), GORM model definitions (`Models`), inter-plugin services (`ctx.Provide`), embedded help docs. | Scoped database operations (`ctx.DB`), AES-256-GCM encryption (`ctx.Encrypt`), safe outbound HTTP (`safehttp`), workspace settings. | No direct `internal/*` imports; user-influenced outbound URLs must use `safehttp` (blocks SSRF/DNS rebinding); write idempotency. |
| **Agent / MCP Tool Execution** | Protocol boundary (stdio / SSE) | **Constrained / Capability-Governed** | MCP tool definitions (`plugin.MCPProvider`), structured LLM tool invocation, input schema validation, formatted execution responses. | Strictly restricted to registered tool handler scopes; operates within request tenant context. | Explicit parameter schemas; execution audit logging; no arbitrary process spawning or unvalidated disk access. |
| **Client Dashboard (React / SDK)** | Browser sandbox | **Zero-Trust Client** | Interactive UI rendering, client-side routing, ephemeral UI state, accessibility (a11y), responsive glass theme design system. | Authenticated JSON HTTP APIs (`/api/...`, `/api/x/...`), session cookies, lazy-loaded chunk assets. | Route-level `PluginGate` degradation (403/404); zero access to server secrets or raw DB queries. |

For development conventions and architectural rules, see [Developer Conventions](/developers/conventions/).

## 6. File Map

Core codebase layout:
- `server/plugin/plugin.go` — Backend plugin contract & `Context`.
- `server/app/app.go` — Server wiring, `Use()`, middleware, and HTTP listeners.
- `packages/plugin-sdk/` — Public frontend SDK package (contracts + UI components).
- `web/src/plugin-sdk/` — Dashboard facade re-exporting the SDK.
- `web/octarq.plugins.json` — Frontend plugin composition manifest.
- `web/plugins-manifest.ts` — Vite plugin generating `#octarq-plugins`.
- `web/src/plugins/PluginRoutes.tsx` — Route renderer & degradation boundary.
- `web/src/plugins/core/` — Built-in core feature UI plugins.
- `server/examples/plugin-hello/` — Reference plugin implementation.
- `website/src/content/docs/` — Documentation site source.
