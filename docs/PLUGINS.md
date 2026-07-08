# Writing a octarq plugin

octarq is extended by **plugins**, not forks. A plugin is a self-contained feature
with two halves that mirror each other:

- a **Go module** implementing the backend contract `plugin.Plugin`, and
- a **JS package** implementing the frontend contract `UIPlugin` (from
  `@octarq-org/plugin-sdk`).

The commercial build (octarq-pro) is nothing more than octarq-core plus a set of these
plugins. Anything it can do, a community plugin can do the same way. The working
reference for everything below is [`examples/plugin-hello`](../examples/plugin-hello),
a minimal full-stack plugin you can copy.

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
- **Frontend** — the host's build-time *injection module* calls
  `registerUIPlugin(fooPlugin)`. A build that never imports your package never
  ships a byte of your UI (the page lands in its own lazy chunk that is only
  referenced when composed in). See `web/src/plugins/index.ts` (open-source: empty)
  vs `web/src/plugins/index.pro.ts` (commercial: registers plugins), selected by
  the `#octarq-plugins` build alias.

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

Optionally implement `MenuProvider` to contribute a sidebar entry:

```go
type MenuProvider interface { Menus() []MenuItem }
```

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
        {ID: "hello", Label: "Hello", Path: "/hello", Icon: "👋", Category: "operations"},
    }
}
```

Key rules:

- **Every route is auto-gated.** The host wraps your mux so that if the *hello*
  feature is disabled for the caller's workspace, the app answers **404** before
  your handler runs. That 404 is exactly the state the frontend renders its
  neutral "not in this build" fallback for. You don't write that check.
- **License-gate Pro routes with 402.** If a route needs a paid tier, return
  **402 Payment Required** when the license lacks it; the frontend shows the
  upsell. (octarq-pro's plugins use `lic.HasTier(...)`.)
- **Never import octarq's `internal/*`.** Everything a plugin needs is on
  `plugin.Context`: `DB`, `Guard`, `Encrypt`/`Decrypt` (AES-256-GCM, for secrets
  at rest — never store plaintext), `Audit`, `Notify`, `SendMail`, `OnEmail`
  (inbound-mail hook), `DNS`, `UserID`/`OrgID`, and
  `GetWorkspaceSetting`/`SetWorkspaceSetting`. Non-startup config belongs in the
  shared `settings` table via those helpers, not env vars.
- Add compile-time assertions: `var _ plugin.Plugin = Plugin{}`.

## The frontend half — `UIPlugin`

```ts
interface UIPlugin {
  name: string;                 // match the Go Plugin.Name()
  routes: { path: string; Component: LazyPage }[];   // LazyPage = React.lazy(...)
  menu?: PluginMenuItem[];      // same shape as the backend MenuItem
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
  menu:  [{ id: "hello", label: "Hello", path: "/hello", icon: "👋", category: "operations" }],
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
- **Handle 402 and 404.** On **402** render `LockedFeature` (upsell); on **404**
  (plugin/endpoint absent in this build) render a neutral note. `lockedFallback`
  is the component the route boundary degrades to if a page chunk fails.
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

Frontend — the host's injection module registers your UI plugin (this is the
`#octarq-plugins` target; octarq-pro provides its own):

```ts
import { registerUIPlugin } from "@octarq-org/plugin-sdk";
import { helloPlugin } from "@acme/octarq-plugin-hello";
registerUIPlugin(helloPlugin);
```

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
- [ ] Backend routes registered on the passed `Mux`; secrets via `ctx.Encrypt`.
- [ ] Pro-only routes return **402** without the tier; you rely on the host's
      auto-**404** for the disabled-feature case.
- [ ] Pages are `React.lazy`; UI built from `@octarq-org/plugin-sdk`; 402/404 handled.
- [ ] i18n keys live under your `name` namespace.
- [ ] `go build ./...` and `pnpm build` are green; `go:embed` produces one binary.
