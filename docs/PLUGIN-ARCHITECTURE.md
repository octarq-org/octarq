# Plugin architecture & commercialization (design + status)

Reference for how octarq's plugin system is built and how the commercial build
(octarq-pro) composes on top of the open-source core **without forking**. For a
plugin *author's* how-to, see [PLUGINS.md](PLUGINS.md); this doc is the
architecture + current-status handoff.

- **octarq** — the open-source core (`github.com/octarq-org/octarq`), MIT.
- **octarq-pro** — the private commercial build; consumes octarq as a Go module
  and mounts Pro features as plugins. `go.work` wires `.` + `../octarq` locally.

## 1. Principle: symmetric, no-fork plugins

A feature is a **plugin** with two mirror halves composed into the core, never a
fork:

- **Backend** — a Go module implementing `plugin.Plugin`.
- **Frontend** — a JS package implementing `UIPlugin` (from the shared SDK).

Both are composed **at build time** (not runtime): compile-time Go interface
implementation + build-time frontend registry injection. One binary, `go:embed`.
This fits the single-binary, self-hosted product and keeps the OSS/commercial
line clean: the OSS build ships the plugin *page shells* that gracefully degrade
(402 upsell / 404 neutral); the commercial build injects the real pages.

## 2. Backend contract (`plugin/plugin.go`)

```go
type Plugin interface {
    Name() string                 // stable id; matches the UIPlugin.name
    Models() []any                // GORM models (migrated for the plugin)
    Mount(mux Mux, ctx *Context)  // register HTTP routes
}
// optional:
type MenuProvider interface { Menus() []MenuItem }
```

- The host calls `app.Use(p)` before `app.Run()`. Every plugin route is
  **auto-gated** by `gatedMux` (app/app.go): if the feature is disabled for the
  caller's workspace it answers **404** before the handler runs.
- Pro routes **license-gate** with **402** (`lic.HasTier(...)`) so the frontend
  shows an upsell.
- Plugins never import `internal/*`; everything they need is on `plugin.Context`
  (DB, Guard, Encrypt/Decrypt, Audit, Notify, SendMail, OnEmail, DNS,
  Get/SetWorkspaceSetting, …).

## 3. Frontend contract & build-time composition

`packages/plugin-sdk/` (published as **`@octarq-org/plugin-sdk`**) owns:

```ts
interface UIPlugin {
  name: string                 // matches Go Plugin.Name()
  routes: { path; Component: LazyPage }[]   // React.lazy pages
  menu?: PluginMenuItem[]       // same shape as backend MenuItem → areaForCategory
  i18n?: { en; zh }             // merged under the plugin's `name` namespace
  lockedFallback?: Component<{ status: number }>   // 402/404 degrade
}
registerUIPlugin(p); uiRoutes(); uiMenus(); uiPluginI18n()
```

**The injection seam is a manifest** (in the core `web/`) — WHICH plugins a build
ships is *data*, not code:

- `web/octarq.plugins.json` — the plugin **manifest**: a list of the UI plugins
  composed into this build. Each entry is a package specifier (its default export
  is the UIPlugin) or `{ from, import }` for a named/local export. The committed
  file is the **OSS default edition**: it lists the example plugin
  (`@acme/octarq-plugin-hello`), so the plugin system works out of the box.
- `web/plugins-manifest.ts` — a Vite plugin that serves the `#octarq-plugins`
  virtual module, generated from the active manifest (`import` + `registerUIPlugin`
  for each entry). `web/src/main.tsx` imports `#octarq-plugins` for its side
  effects. This replaces the old two-file seam (`index.ts` / `index.pro.ts`) and
  the `VITE_OCTARQ_PLUGINS` switch.
- **Choosing an edition** = pointing at a different manifest, highest precedence
  first: `OCTARQ_PLUGINS` env (inline JSON array — **dynamic CI injection**, no
  file to edit) › `OCTARQ_PLUGINS_MANIFEST` env (path to a manifest file — a
  commercial build ships its own; octarq-pro points here) › the committed
  `web/octarq.plugins.json`.
- Result: a build never references a plugin its manifest doesn't name (verified:
  the licenses page markers `LicensesPage`/`getApiIssued`/`No licenses issued`
  are **absent** from the OSS/example build, **present** only when a manifest
  composes `@octarq-org/plugin-issuer`).

`web/src/App.tsx` renders `pluginRouteElements()` and folds `uiMenus()` into the
sidebar via the existing `areaForCategory`; a route with no registered plugin
404-degrades (neutral note). Licenses is the reference plugin, now published as
the standalone package `@octarq-org/plugin-issuer` (octarq-pro `packages/`).

## 4. Shared UI (`@octarq-org/plugin-sdk`)

The SDK re-exports the shared component library plugins build against, so a
plugin page matches the app and gets accessibility for free:

- Backed by **shadcn/ui + Base UI** (`@base-ui/react`) with the dark "glass"
  theme (chosen for Tailwind-4 support, single-binary embed, MIT, AI-codegen).
- `cn()` (clsx + tailwind-merge); Button/Badge via `class-variance-authority`;
  Modal→Base UI Dialog, Toggle→Base UI Switch (a11y/focus-trap/scroll-lock);
  plus Input/Textarea/Select/Tabs/Tooltip/Table/Skeleton.
- **Dependency inversion:** the app consumes the package via a facade
  (`web/src/plugin-sdk/`) that re-exports it; i18n-coupled bits (`Code`,
  `LockedFeature`) stay in the app facade. ~50 `../ui` importers unchanged.
- For AI/LLM UIs (Inbox AI) the recommended layer is **assistant-ui** (same
  shadcn/Tailwind DNA) — not yet adopted.

## 5. Commercial embedding — how octarq-pro serves Pro pages

The core's binary embeds `webembed/dist` (OSS dashboard, empty registry). The
commercial build overrides it:

1. **`app.WithWebFS(fs.FS)`** (core `app/app.go`) — injection point; defaults to
   the embedded OSS FS when unset.
2. **`OCTARQ_WEBEMBED_OUT`** (core `web/vite.config.ts` + `vite.portal.config.ts`
   + build script) — makes the admin+portal build outDir overridable so the
   commercial build reuses the exact same build; default unchanged.
3. **octarq-pro `webembed/`** — its own package embedding a dashboard built
   against octarq-pro's plugin manifest into `octarq-pro/webembed/dist` (via
   `make web`, which runs `OCTARQ_WEBEMBED_OUT=$(CURDIR)/webembed/dist
   OCTARQ_PLUGINS_MANIFEST=$(CURDIR)/octarq.plugins.json pnpm build` against
   `../octarq/web`). `main.go` calls `a.WithWebFS(webembed.FS())`. CI's
   `dashboard.yml` builds and commits this dist with a Packages token so the
   private plugin packages resolve.

The committed pro dist means `go build`/Docker embed it with no cross-repo
frontend build; `make web` (or `dashboard.yml`) regenerates it when the core
dashboard or the plugin set changes.

**Proven end-to-end:** OSS dashboard has no Pro pages (404-degrade); octarq-pro's
embedded dashboard renders the real licenses page.

## 6. A Pro feature's journey (end to end)

1. Backend plugin (`plugin.Plugin`) mounts routes, 402-gates on tier — lives in
   octarq-pro (or core for a community plugin).
2. Frontend page (`UIPlugin`) built from `@octarq-org/plugin-sdk`, handles 402/404.
   The end state is a standalone package (`octarq-pro/packages/plugin-<feat>/`,
   consuming the SDK + `@octarq-org/api-client`) named in the Pro manifest;
   pages still mid-migration live in core `web/src/plugins/<feat>/` and are
   bridged by a `{ from: "./src/plugins/<feat>", import }` manifest entry.
3. OSS build: manifest omits it → page 404-degrades (or shows upsell on 402).
4. Commercial build: octarq-pro's manifest names it → composed into the dist;
   octarq-pro embeds that dist via `WithWebFS`; the licensed backend serves it.

## 7. Publishing the SDK

`@octarq-org/plugin-sdk` → **GitHub Packages** via changesets:
- Root `pnpm-workspace.yaml` (`packages/*`), `.changeset/`, `.github/workflows/publish-sdk.yml`.
- Package `publishConfig` → `npm.pkg.github.com`, scope `@octarq-org` (= GitHub org).
- Consumer `.npmrc`: `@octarq-org:registry=https://npm.pkg.github.com`. See
  [PUBLISHING.md](PUBLISHING.md).
- Requires the `octarq-org` GitHub org + Packages enabled; not yet published.

## 8. File map

Core (octarq):
- `plugin/plugin.go` — backend contract + `Context`.
- `app/app.go` — `Use`, `gatedMux`, `WithWebFS`, CSRF wrap, server wiring.
- `packages/plugin-sdk/` — the SDK package (contract + shadcn UI).
- `web/src/plugin-sdk/` — app-side facade re-exporting the package.
- `web/octarq.plugins.json` — the plugin manifest (OSS default edition).
- `web/plugins-manifest.ts` — Vite plugin generating the `#octarq-plugins` virtual module from the manifest.
- `web/src/plugins/{PluginRoutes.tsx,<feat>/}` — route renderer + Pro pages still mid-migration.
- `web/vite.config.ts` / `vite.portal.config.ts` — `octarqPlugins()`, `OCTARQ_WEBEMBED_OUT`.
- `examples/plugin-hello/web/` — the example plugin, packaged as `@acme/octarq-plugin-hello` (OSS default).
- `docs/{PLUGINS.md,PUBLISHING.md,ACCESSIBILITY.md}`.

octarq-pro:
- `main.go` — `app.New().WithWebFS(webembed.FS()).Use(...)`.
- `webembed/` — embeds the Pro-injected dashboard; `make web` rebuilds.

## 9. Status & next steps (handoff)

**Done (all local, verified green — go build/test-race, tsc, OSS+Pro builds, example, SDK tsup):**
- ✅ Backend plugin system (pre-existing) + `gatedMux` + `Context`.
- ✅ `@octarq-org/plugin-sdk` extracted (contract + registry + shadcn/Base-UI shared UI incl. new components); app dependency-inverted via facade.
- ✅ Build-time injection seam (`#octarq-plugins` / `VITE_OCTARQ_PLUGINS`); byte-level OSS exclusion verified; licenses is the reference plugin.
- ✅ Commercial embedding: `app.WithWebFS` + `OCTARQ_WEBEMBED_OUT` + octarq-pro `webembed`; octarq-pro serves real Pro pages end-to-end.
- ✅ Publish pipeline scaffolded (changesets + GitHub Packages + PUBLISHING.md).
- ✅ Theme tokens (`@theme`) + tw-animate-css + a11y audit (ACCESSIBILITY.md).
- ✅ Full led→octarq rebrand (both repos); history linearized (0 merge commits).
- ✅ Security reconcile onto main: OAuth registration gate, gothic cookie hardening, link-target scheme validation, go 1.25.11, CSRF guard+tests, SSRF-guarded webhook/notification delivery (`OCTARQ_ALLOW_PRIVATE_WEBHOOKS`).

**Pending — gated on external setup:**
- ⏳ **Create the `octarq-org` GitHub repos** (`octarq`, `octarq-pro`), then `git push -u origin main` (both). Remotes already point there. Neither repo is pushed.
- ⏳ After the core is pushed: in octarq-pro run `go get github.com/octarq-org/octarq@latest` to bump the stale core pseudo-version (`v0.1.2-0.2026070503…` predates the rename; builds locally only via `replace => ../octarq`).
- ⏳ Actually publish `@octarq-org/plugin-sdk` (needs the org + Packages + "Allow Actions to create PRs").

**Optional follow-ups (not started):**
- Adopt shadcn wrappers for the *rest* of `web/src/ui` (only the interactive primitives were migrated; presentational ones stay styled-divs).
- Adopt **assistant-ui** for Inbox AI / LLM chat surfaces.
- Port the `security-hardening` branch's **i18n improvements** (portal 中文化, plain-language copy) to main — UX only, non-security.
- Address the ACCESSIBILITY.md component-edit recommendations (keyboard-enable clickable `<code>`/StatCard, dropdown menu semantics, `<MotionConfig reducedMotion>`), and WCAG-AA contrast on the faint `text-white/3x` tones.
- Migrate the app to depend on the *published* `@octarq-org/plugin-sdk` (currently a workspace source-path facade).
