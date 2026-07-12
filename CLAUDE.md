# Working in this repo (octarq)

Conventions for how I want changes made here. Not feature docs — see README.

## Toolchain

- **Go 1.25**, standard-library `http.ServeMux`. Backend is pure-Go (no cgo).
- **Frontend in `web/`**: Vite + React + TS + Tailwind. Package manager is
  **pnpm 9** (`packageManager: pnpm@9.15.4`).
- **Never use npm.** Use pnpm.
- **Do NOT bump to pnpm 10/11 here.** They treat esbuild's build script as a fatal
  `ERR_PNPM_IGNORED_BUILDS` and break CI. `packageManager` lives in `web/`, not the
  repo root, so CI's `pnpm/action-setup` must pin `version: 9.15.4`.

## Running it

- **Always start dev servers with `--host`** so they're reachable on the network.
  Frontend: `cd web && pnpm dev --host`.
- Backend: `OCTARQ_SECRET_KEY=dev OCTARQ_ADMIN_PASSWORD=dev go run .` (serves `:8080`).
- Full build: `make release`.

## Verify before saying "done"

Run these and make sure they pass — don't claim a change works on inspection alone:

- `go build ./...`
- `go test ./... -race`
- `gofmt -w` (keep gofmt-clean; match surrounding style)
- in `web/`: `npx tsc --noEmit`

**Prefer pushing and relying on CI over spinning up Docker locally** to verify — for
this repo that's the faster loop. Only commit/push when asked.

**Never build or commit `webembed/dist` manually.** CI auto-commits a fresh build
on every push to main. It must stay tracked in git — led-pro consumes led as a Go
module and gets the embedded dashboard from the module zip.

## Code conventions

- **Single source of truth — derive, don't duplicate.** E.g. `areaForPath` in
  `web/src/App.tsx` is derived from `STATIC_AREAS`; never reintroduce a parallel
  hardcoded path→area mapping. When you catch this kind of duplication, collapse it.
- **Don't cram.** Split overgrown components/config into focused modules rather than
  piling more into one (the settings pages and sidebar areas follow this).
- **Frontend Pro/optional features degrade gracefully**: a page hitting a plugin
  endpoint should handle **402** (show an upsell, e.g. `LockedFeature`) and **404**
  (the plugin isn't in this build — show a neutral note), never a raw error.
- **Sidebar menus & routes**: **every business page is a UIPlugin** — core
  features live in `web/src/plugins/core/` (always composed, imported from
  `main.tsx` before the `#octarq-plugins` manifest module), Pro/third-party
  ones come from the manifest. All flow through the same registry
  (`registerUIPlugin` → `uiRoutes()`/`uiMenus()`); the shell (`App.tsx`) owns
  only auth, settings, org handling, Overview and the plugin pipeline — never
  add a hardcoded business route there. `STATIC_AREAS` holds only **area/group
  shells** (label + order, items `[]`); a menu's `category` must equal its
  group label (`areaForCategory` places it), registration order is item order
  within a group, and empty groups/areas are dropped at runtime. The
  dynamic-fallback group is **"More"**, never "Plugin(s)". Plugins can declare
  new top-level areas via `UIPlugin.areas`; icons are string keys resolved by
  the single `PLUGIN_ICONS` table in `shell/areas.tsx`.

## Sandbox note

If a Vite build fails with "service was stopped", point `ESBUILD_BINARY_PATH` at the
real Mach-O binary under `node_modules/.pnpm/@esbuild+darwin-arm64@*/.../bin/esbuild`
(not the JS shim).
