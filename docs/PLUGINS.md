# Writing a octarq plugin

octarq is extended by **plugins**, not forks. A plugin is a self-contained feature
with two halves that mirror each other:

- a **Go module** implementing the backend contract `plugin.Plugin`, and
- a **JS package** implementing the frontend contract `UIPlugin` (from
  `@octarq-org/plugin-sdk`).

Anyone can extend Octarq using this exact same plugin mechanism. Octarq's own core
pages (links, mail, DNS, abuse, audit) and commercial Pro modules are built as
plugins on top of this public contract — a community plugin can do everything an
official or commercial plugin can do. The shell owns only auth, settings, org
handling, Overview, and the plugin pipeline. The working reference for everything
below is [`examples/plugin-hello`](../examples/plugin-hello), a minimal full-stack
plugin you can copy.

```
your-plugin/
├── go.mod                 # a Go module: github.com/you/octarq-plugin-foo
├── foo.go                 # implements plugin.Plugin (+ optional MenuProvider)
└── web/
    ├── index.ts           # implements UIPlugin  (@octarq-org/plugin-sdk)
    └── Page.tsx           # your lazy-loaded page(s)
```

## How composition works (and what it means for trust)

There is **no runtime plugin loading**. Both halves are composed at build time:

- **Backend** — a host `main.go` imports your module and calls `app.Use(foo.Plugin{})`
  before `app.Run()`. The result is a single Go binary; `go:embed` bakes the
  frontend into it. (This is the same model as Caddy/`xcaddy`: pick your plugins,
  build a binary.)
- **Frontend** — the host lists your package in its **plugin manifest**
  (`web/octarq.plugins.json`); the build generates the `#octarq-plugins` module
  that imports it and calls `registerUIPlugin(fooPlugin)`. A build whose manifest
  doesn't name your package never ships a byte of your UI (the page lands in its
  own lazy chunk, only referenced when composed in). The OSS default manifest
  lists the example plugin; custom builds or commercial editions point at their own
  manifest via `OCTARQ_PLUGINS_MANIFEST` / `OCTARQ_PLUGINS` (see `web/plugins-manifest.ts`).

Because a compiled-in plugin runs **in-process with full access** (DB, secrets,
network), this model fits a **curated / operator-opt-in** ecosystem: the operator
chooses which plugins to build in, and you review what you ship. It is **not** a
sandbox for untrusted third-party code. If you ever need to run untrusted
plugins, that requires process/WASM isolation — a different mechanism than this.

## The backend half — `plugin.Plugin`

```go
type Plugin interface {
    Name() string                     // stable id; match the UIPlugin.name
    Models() []any                    // GORM models this plugin owns (migrated for you)
    Mount(mux Mux, ctx *Context)      // register HTTP routes
}
```

Beyond the required interface, a plugin opts into capabilities by implementing
optional interfaces:

```go
type MenuProvider interface { Menus() []MenuItem }      // sidebar entries
type Starter interface { Start(ctx context.Context) }   // background work
// also: MCPProvider, OpenAPIContributor, Describer — see plugin/plugin.go
```

`Start` runs in its own goroutine only **after every plugin has mounted**, so it
is the earliest safe point to look up services from other plugins.

From `examples/plugin-hello/hello.go`:

```go
func (Plugin) Name() string { return "hello" }
func (Plugin) Models() []any { return nil } // stateless example

func (Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
    mux.Handle("GET /api/hello/ping", ctx.Guard(http.HandlerFunc(
        func(w http.ResponseWriter, r *http.Request) { /* ... */ })))
}

func (Plugin) Menus() []plugin.MenuItem {
    return []plugin.MenuItem{
        {ID: "hello", Label: "Hello", Path: "/hello", Icon: "👋", Category: "Workspace"},
    }
}
```

Key rules:

- **Every route is auto-gated.** The host wraps your mux so that if the *hello*
  feature is disabled for the caller's workspace, the app answers **404** before
  your handler runs. That 404 is exactly the state the frontend renders its
  neutral "not in this build" fallback for. You don't write that check.
- **License-gate paid routes with 402 (optional).** If a plugin feature requires a
  paid tier or commercial license, return **402 Payment Required** when the license
  lacks it; the frontend `PluginGate` automatically catches 402 responses and renders
  an upsell or custom locked view.
- **Never import octarq's `internal/*`.** Everything a plugin needs is on
  `plugin.Context`: `DB`, `Guard`, `Encrypt`/`Decrypt` (AES-256-GCM, for secrets
  at rest — never store plaintext), `Audit`, `Notify`, `SendMail`, `OnEmail`
  (inbound-mail hook), `DNS`, `UserID`/`OrgID`, and
  `GetWorkspaceSetting`/`SetWorkspaceSetting`. Non-startup config belongs in the
  shared `settings` table via those helpers, not env vars.
- **Talk to other plugins through the service registry**, never by importing
  them: the provider calls `ctx.Provide("<pluginName>.<service>", svc)` during
  `Mount` (e.g. `"hello.greeter"`); a consumer resolves it **lazily** — in
  `Start` or per-request, never in its own `Mount` — with the typed helper
  `plugin.LookupAs[T](ctx, name)` and degrades gracefully when it's absent.
  Providing the same name twice is a startup error.
- **Own your tables.** All models (core + every plugin) are AutoMigrated once,
  after registration; a startup preflight fails if two different plugin model
  types claim the same table. Mirroring an *existing core* table with a local
  struct (`TableName()` override) is the allowed convention for reading core
  data without importing `internal/models`.
- **Pair every interface you implement with a compile-time assertion** —
  optional capabilities are detected by runtime type assertion, so a typo'd
  method silently never runs without these:

  ```go
  var (
      _ plugin.Plugin       = Plugin{}
      _ plugin.MenuProvider = Plugin{}
      _ plugin.Starter      = Plugin{} // for each optional interface you claim
  )
  ```

## The frontend half — `UIPlugin`

```ts
interface UIPlugin {
  name: string;                 // match the Go Plugin.Name()
  routes: { path: string; Component: LazyPage;       // LazyPage = React.lazy(...)
            requiredTier?: string; requiredRole?: string }[];
  menu?: PluginMenuItem[];      // same shape as the backend MenuItem (+ requiredRole)
  widgets?: { slot: string; Component: LazyPage; order?: number }[];
  areas?: { id: string; title: string; subtitle?: string; icon?: string }[];
  i18n?: { en: {...}; zh: {...} };
  lockedFallback?: ComponentType<{ status: number }>;
}
```

From `examples/plugin-hello/web/index.ts`:

```ts
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";

export const helloPlugin: UIPlugin = {
  name: "hello",
  routes: [{ path: "/hello", Component: lazy(() => import("./Page")) }],
  menu:  [{ id: "hello", label: "Hello", path: "/hello", icon: "👋", category: "Workspace" }],
  i18n:  { en: { pageTitle: "Hello Plugin", /* ... */ }, zh: { /* ... */ } },
};
```

Key rules:

- **Wrap each page in `React.lazy`.** This is what makes an uncomposed build ship
  none of your bytes, and gives each page its own async chunk.
- **Build your UI from `@octarq-org/plugin-sdk`.** It re-exports the shared component
  library — `GlassCard`, `Button`, `Badge`, `Field`, `Modal`, `Toggle`, `Empty`,
  `PageHeader`, `LockedFeature`, `useTranslation`, … These are now backed by
  shadcn / Base UI primitives (accessible, keyboard-operable) while carrying
  octarq's glass theme, so your page matches the app and gets a11y for free. Import
  by name from `@octarq-org/plugin-sdk`, never reach into app-internal paths.
- **Gated states are centralized in `PluginGate`.** Every plugin route element is
  wrapped in it: **402** → the upsell (`lockedFallback`, or the SDK's
  `LockedFeature` by default), **403** → a neutral access-denied note, **404**
  and chunk-load/render failures → the neutral "not part of this build" note.
  A page can degrade declaratively via `usePluginGate().degrade(err.status)` from
  a data-fetch catch, or keep handling 402/404 itself — the gate is the safety
  net, never a raw error.
- **`category` must equal the sidebar group label** it joins (e.g. `"Workspace"`,
  `"Marketing"`, `"Network"`); a category with no matching group creates one,
  routed to a top-level area by `areaForCategory`'s keywords. A plugin can also
  declare a **new top-level area** (`areas`) and point menu categories at its
  id/title. Icons are string keys resolved by the app's single `PLUGIN_ICONS`
  table (unknown menu icons render literally, e.g. an emoji).
- **`requiredRole`/`requiredTier` are advisory UX only.** The host hides menu
  entries and pre-renders access-denied for users below `requiredRole`
  (member < admin < owner, instance admin bypasses) — but the server stays
  authoritative: enforce with 403/402 in your backend half.
- **Widgets** render into named `<ExtensionSlot>`s — the Overview page renders
  slot `"home-overview"` — ordered by ascending `order`.
- **i18n namespace = your `name`.** Your `i18n.en`/`i18n.zh` merge under
  `"<name>"`, so a `pageTitle` key is read as `t("hello.pageTitle")`. This keeps
  plugin translations from colliding with core or each other.
- **`path` is an absolute admin path** (e.g. `/hello`), rendered under the same
  `/admin` basename as core routes. Keep it in sync with the backend menu's `Path`.

## Wiring it into a host build

Backend (`main.go` of your custom binary):

```go
app, _ := app.New()
app.Use(hello.Plugin{})   // + any other plugins
app.Run(ctx)
```

Frontend — add your package to the host's plugin manifest
(`web/octarq.plugins.json`); the build imports and registers it for you:

```json
{ "plugins": ["@acme/octarq-plugin-hello"] }
```

Each entry is a package specifier (its default export is the UIPlugin) or
`{ "from": "<spec>", "import": "<namedExport>" }`. To compose a different set
without editing the file, pass `OCTARQ_PLUGINS='["@acme/octarq-plugin-hello"]'`
or point `OCTARQ_PLUGINS_MANIFEST` at another manifest (custom or commercial builds
use this to compose their editions).

Build the frontend (`pnpm build` bakes it into `webembed/dist`), then
`go build` the binary. One binary, both halves, no fork, no runtime fetch.

## Distributing a plugin

A plugin is **one repo** with the two halves. In a real distribution:

- The Go module is `go get`-able; a host adds it to its build.
- The `web/` package is published to npm (e.g. `@acme/octarq-plugin-hello`) with
  `@octarq-org/plugin-sdk` and `react` as **peer** dependencies; the host imports it by
  name and registers it in its injection module.

The example's `web/tsconfig.json` maps the SDK/React locally only because it
lives inside this repo without its own `node_modules`; a published package
resolves them as normal peers and needs no such mapping.

## Checklist

- [ ] `Plugin.Name()` and `UIPlugin.name` are identical.
- [ ] A compile-time `var _ plugin.X = Plugin{}` assert for **every** interface
      the plugin implements (Plugin + each optional one).
- [ ] Backend routes registered on the passed `Mux`; secrets via `ctx.Encrypt`;
      cross-plugin services via `ctx.Provide` / lazy `plugin.LookupAs`.
- [ ] Paid/tiered routes return **402** when unlicensed; you rely on the host's
      auto-**404** for the disabled-feature case.
- [ ] Pages are `React.lazy`; UI built from `@octarq-org/plugin-sdk`; 402/404 handled.
- [ ] i18n keys live under your `name` namespace.
- [ ] `go build ./...` and `pnpm build` are green; `go:embed` produces one binary.

