---
title: Writing a Plugin
description: Guide to writing a custom Octarq plugin.
sidebar:
  order: 1
  group:
    label: "Build a Plugin"
---


Octarq is extended by plugins. A plugin consists of:

- A **Go module** implementing `plugin.Plugin`
- A **JS package** implementing `UIPlugin` (from `@octarq/plugin-sdk`)

---

## 1. Directory Structure

A minimal plugin repository follows this structure:

```
your-plugin/
├── go.mod                 # Go module (e.g., github.com/you/octarq-plugin-hello)
├── hello.go               # Implements plugin.Plugin (+ optional MenuProvider, MCPProvider)
└── web/
    ├── index.ts           # Implements UIPlugin (@octarq/plugin-sdk)
    └── Page.tsx           # React UI page
```

---

## 2. Backend Implementation (`plugin.Plugin`)

Every plugin implements `plugin.Plugin` and optionally registers routes, models, menus, or MCP tools.

```go
package hello

import (
    "net/http"
    "github.com/octarq-org/octarq/plugin"
)

type Plugin struct{}

// Name returns a unique, stable ID for the plugin.
func (Plugin) Name() string { return "hello" }

// Models returns GORM model structs owned by this plugin.
func (Plugin) Models() []any { return nil }

// Mount registers HTTP endpoints on the host router.
func (Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
    mux.Handle("GET /api/hello/ping", ctx.Guard(http.HandlerFunc(
        func(w http.ResponseWriter, r *http.Request) {
            w.Write([]byte(`{"message": "pong"}`))
        },
    )))
}

// Menus defines sidebar entries provided by this plugin.
func (Plugin) Menus() []plugin.MenuItem {
    return []plugin.MenuItem{
        {ID: "hello", Label: "Hello", Path: "/hello", Icon: "👋", Category: "Workspace"},
    }
}

// Compile-time interface assertions
var (
    _ plugin.Plugin       = Plugin{}
    _ plugin.MenuProvider = Plugin{}
)
```

Key backend rules:

- **Every route is auto-gated.** The host wraps your mux so a feature disabled for the caller's workspace answers `404` before your handler runs.
- **License-gate paid routes with `402`.** Return `402 Payment Required` when the license lacks the tier; the frontend `PluginGate` turns it into an upsell.
- **Never import `internal/*`.** Everything a plugin needs is on `plugin.Context`: `DB`, `Guard`, `Encrypt`/`Decrypt` (AES-256-GCM), `Audit`, `Notify`, `SendMail`, `OnEmail`, `DNS`, `UserID`/`OrgID`, `GetWorkspaceSetting`/`SetWorkspaceSetting`.
- **Pair every optional interface with a compile-time assertion** (`var _ plugin.MenuProvider = Plugin{}`). Optional capabilities are detected by runtime type assertion, so a typo'd method silently never runs without these.
- **Own your tables.** Every model (core + plugins) is AutoMigrated once at startup; a preflight fails if two plugin model types claim the same table. Mirror an existing core table with a local struct (`TableName()` override) to read core data without importing `internal/models`.

---

## 3. Frontend Implementation (`UIPlugin`)

The frontend uses `@octarq/plugin-sdk` to register routes, sidebar items, widgets, and translation dictionaries.

```ts
import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

export const helloPlugin: UIPlugin = {
  name: "hello",
  routes: [
    {
      path: "/hello",
      Component: lazy(() => import("./Page")),
    },
  ],
};
```

Key frontend rules:

- **Wrap every page in `React.lazy`.** An uncomposed build ships none of your bytes; each page gets its own async chunk.
- **Build your UI from `@octarq/plugin-sdk`.** It re-exports the shared library (`GlassCard`, `Button`, `Field`, `Modal`, `Toggle`, `LockedFeature`, `useTranslation`, …). Import by name, never reach into app-internal paths.
- **`PluginGate` wraps every plugin route.** `402` → upsell (`lockedFallback` or the SDK's `LockedFeature`), `403` → access-denied note, `404`/chunk failure → neutral "not in this build" note. Degrade declaratively via `usePluginGate().degrade(err.status)`; the gate is the safety net, never a raw error.
- **`category` equals the sidebar group label it joins.** A `category` with no matching group creates one via `areaForCategory`. A plugin can declare a **new top-level area** (`areas`) and point menu categories at its id or group labels — never at its `title` (display text).
- **A settings page's `Path` is `/settings/<menu id>`.** The last segment is the menu's **`ID`**, not its `Label`. The Go `Path` and frontend `UIRoute.path` must match exactly — `PluginGate` compares them to tell "operator disabled this" apart from "this build doesn't have it".
- **`requiredRole`/`requiredTier` are advisory UX only.** The host hides menu entries and pre-renders access-denied below `requiredRole` — but the server stays authoritative: enforce with `403`/`402` in your backend.
- **i18n namespace = your `name`.** `i18n.en`/`i18n.zh` merge under `"<name>"`, so a `pageTitle` key is read as `t("hello.pageTitle")`.

---

## 4. Instance vs Tenant Scope

Octarq runs at two scopes, and a plugin page belongs to exactly one of them:

- **Tenant scope** — one per workspace. The UI lives in the `/admin` shell; the
  sidebar entry comes from `plugin.MenuProvider` (`plugin/plugin.go`), is served
  by `GET /api/menus`, and is gated per workspace: a workspace that disables the
  feature stops seeing it. The frontend page is a `UIPlugin.routes` entry.
- **Instance scope** — one per deployment. The UI lives in the `/instance`
  console; the rail entry comes from `plugin.InstanceMenuProvider`
  (`plugin/plugin.go`), is served by the instance-admin-gated
  `GET /api/instance/menus` endpoint, and has no per-workspace toggle — the
  entry is announced by the deployment or it isn't. The frontend page is a
  `UIPlugin.instanceRoutes` entry.

Pick the scope with the deciding question:

> A config that exists once per deployment → instance scope; one that exists per
> workspace → tenant scope. The same page must never be reachable from both
> shells.

To pair the two halves of an instance-scope page, implement
`InstanceMenuProvider` on the Go side and register `instanceRoutes` on the JS
side, with matching `Path`/`path` values:

```go
var _ plugin.InstanceMenuProvider = (*Plugin)(nil)

func (p *Plugin) InstanceMenus() []plugin.MenuItem {
    return []plugin.MenuItem{{ID: "sso", Label: "SSO", Path: "/sso", Icon: "key-round"}}
}
```

```ts
const myPlugin: UIPlugin = {
  name: "my-plugin",
  instanceRoutes: [{ path: "/sso", Component: lazy(() => import("./SsoPage")) }],
};
```

The console renders an entry only when the backend's `/api/instance/menus`
announces it **and** the frontend registers an `instanceRoutes` entry for the
same path — mirroring how the tenant sidebar trusts `/api/menus` (see the
sidebar merge in `web/src/App.tsx`). The rail ignores `Category` and
`RequiredRole`: it is flat, and instance admin is the only gate.

---

## 5. Route Namespace & Idempotent Writes

### Namespace

`http.ServeMux` **panics** on a duplicate pattern, so two plugins claiming the
same path is a boot crash. Octarq catches that before the mux does and refuses
to start with an error naming both plugins — but the only way to be sure your
routes never collide with a future core or Pro route is to stay inside the
namespace reserved for out-of-tree plugins:

```
/api/x/{your-plugin-name}/...
```

This is **enforced** for third-party plugins: an out-of-tree plugin that
registers an `/api/...` route outside `/api/x/{name}/` is refused at startup.
In-tree paths that predate the rule (`/api/domains`, `/api/emails`,
`/api/products`) are deliberately left alone; the bare top-level nouns are
already spoken for, and this is what keeps them from becoming a moving target
for you.

Routes outside `/api/` (a public landing page, an OAuth callback) are not
subject to the rule.

```go
mux.Handle("POST /api/x/hello/greetings", ctx.Guard(createGreeting))
```

### Idempotent writes

Any endpoint that creates a resource, sends a message, or moves money should
accept an `Idempotency-Key`. A client whose request times out mid-flight will
retry, and without a key the retry is a second side effect.

The host provides the mechanism through the service registry; resolve it
lazily and wrap the handlers that need it:

```go
import "github.com/octarq-org/octarq/idempotency"

type middleware = func(http.Handler) http.Handler

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
    idem := func(h http.Handler) http.Handler { return h } // no-op fallback
    if m, ok := plugin.LookupAs[middleware](ctx, idempotency.ServiceName); ok {
        idem = m
    }
    mux.Handle("POST /api/x/hello/greetings", idem(ctx.Guard(createGreeting)))
}
```

Behaviour, once wrapped:

- Requests **without** the header are unaffected — adoption never changes
  existing clients.
- The first request runs; its response is stored per `(workspace, endpoint,
  key)` and replayed for repeats within 24h with `Idempotency-Replayed: true`.
- A repeat arriving while the first is still running gets `409` + `Retry-After`.
- The same key with a **different** body gets `422` — never someone else's
  response.
- `5xx`, `429` and panics release the key, so the client's retry is still
  possible.
- A response too large (>1 MiB) or streamed cannot be stored; the repeat gets
  `409` with `Idempotency-Original-Status` rather than a fabricated empty body.
  The handler still never runs twice.

---

## 6. Inter-Plugin Service Registry

Plugins communicate through an in-memory service registry provided on `plugin.Context`.

- **Provide a service**:
  ```go
  ctx.Provide("hello.service", myServiceInstance)
  ```
- **Lookup a service safely**:
  ```go
  if svc, ok := plugin.LookupAs[MyService](ctx, "hello.service"); ok {
      svc.DoSomething()
  }
  ```

> `Start(ctx context.Context)` from optional `Starter` interface runs in a background goroutine after all plugins have mounted, providing an entry point for inter-plugin initialization.

Services are resolved **lazily** — in `Start` or per-request, never in your own `Mount` — and degrade gracefully when absent. Providing the same service name twice is a startup error.

---

## 7. Ship In-App Help Docs

Ship documentation as a `docs/` directory, embedded and served under `/help/<slug>`:

```go
//go:embed docs
var docs embed.FS

func (p *Plugin) HelpDocsFS() fs.FS { return docs }
```

The naming is the contract: `docs/webhooks.mdx` is a page whose slug is `webhooks`; `docs/webhooks.zh.mdx` is its Chinese translation and never a page of its own. Everything else — title, category, order — is YAML frontmatter in the file, and `category` must be one of the six keys from `plugin.HelpCategories()`. Subdirectories are walked and don't affect slugs. For pages built at runtime, implement `HelpProvider` (`HelpDocs() []plugin.HelpDoc`) instead — a plugin may implement both, and the two are concatenated.

---

## 8. Composition & Building

Octarq plugins are composed at build time (similar to `xcaddy`):

```bash
# Build custom binary with your Go and JS plugin modules
OCTARQ_PLUGINS='[{"go":"github.com/you/octarq-plugin-hello","npm":"@you/octarq-plugin-hello"}]' make plugin-build
```

- **Routes Auto-Gate**: Endpoints return `404` when disabled in workspace settings.
- **AutoMigrate Preflight**: Database tables are resolved and migrated at startup.

---

## 9. Trust model

There is **no runtime plugin loading** — a compiled-in plugin runs **in-process with
full access** (DB, secrets, network). This fits a **curated / operator-opt-in**
ecosystem: the operator chooses which plugins to build in, and you review what you
ship. It is **not** a sandbox for untrusted third-party code; untrusted plugins
would need process/WASM isolation.

---

## 10. Distribution

A plugin is **one repo** with the two halves: the Go module is `go get`-able, and
the `web/` package publishes to npm with `@octarq/plugin-sdk` and `react` as
**peer** dependencies. The working reference is
[`examples/plugin-hello`](https://github.com/octarq-org/octarq/tree/main/examples/plugin-hello).
For publishing the SDK itself, see [Publishing the SDK](/guides/publishing/).

---

## 11. Checklist

- [ ] `Plugin.Name()` and `UIPlugin.name` are identical.
- [ ] A compile-time `var _ plugin.X = Plugin{}` assert for **every** interface (Plugin + each optional one).
- [ ] Backend `/api` routes live under `/api/x/<name>/`; write endpoints accept `Idempotency-Key`.
- [ ] Backend routes registered on the passed `Mux`; secrets via `ctx.Encrypt`; cross-plugin services via `ctx.Provide` / lazy `plugin.LookupAs`.
- [ ] Paid/tiered routes return **402** when unlicensed; rely on the host's auto-**404** for the disabled-feature case.
- [ ] Pages are `React.lazy`; UI built from `@octarq/plugin-sdk`; 402/404 handled.
- [ ] i18n keys live under your `name` namespace.
- [ ] Help pages live in `docs/<slug>.mdx` with a `<slug>.zh.mdx` translation; title/category/order are frontmatter.
- [ ] `go build ./...` and `pnpm build` are green; `go:embed` produces one binary.

