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

**Never build or commit `webembed/dist` manually.** It must stay tracked in git —
octarq-pro consumes octarq as a Go module and gets the embedded dashboard from
the module zip — but keeping it current is CI's job, not yours.

**The refresh happens after the merge.** On a push to main, CI rebuilds the
dashboard and, if the output differs, opens a `chore(web): refresh embedded
dashboard build` PR containing only `webembed/dist`. Merge it as-is. (It has no
checks of its own: PRs opened with `GITHUB_TOKEN` don't trigger workflows. The
source it was built from already passed CI on that commit.)

This used to run on the PR branch instead. The bot-authored push became the new
head and GitHub gated the run at **`action_required`**, so every frontend PR read
as unchecked until someone approved a commit nobody reviews. Don't move it back.

One consequence while you work: on any branch that touches `web/`, the committed
`webembed/dist` is stale until that branch merges. **Running the Go binary alone
will serve the old dashboard.** For local verification run the frontend live —
`cd web && pnpm dev --host` against the backend — rather than trusting dist.

## Code conventions

- **Single source of truth — derive, don't duplicate.** E.g. `areaForPath` in
  `web/src/shell/areas.tsx` is derived from the area/menu data; never reintroduce a
  parallel hardcoded path→area mapping. When you catch this kind of duplication,
  collapse it.
- **Don't cram.** Split overgrown components/config into focused modules rather than
  piling more into one (the settings pages and sidebar areas follow this).
- **Frontend Pro/optional features degrade gracefully**: a page hitting a plugin
  endpoint should handle **402** (show an upsell, e.g. `LockedFeature`) and **404**
  (the plugin isn't in this build — show a neutral note), never a raw error.
- **Sidebar menus come from the Go half, routes from the frontend half.** A
  plugin's `MenuProvider` (`Menus()` → `/api/menus`) is the ONLY source of
  sidebar placement — id, path, label, category, icon, order. `UIPlugin` has no
  `menu` field: the host drops any entry whose path the backend doesn't
  announce (that is what makes a disabled feature disappear), so a
  frontend-declared menu could never stand alone — it was only ever a copy, and
  the copies drifted. Icons on the Go side are **lucide keys** (`link-2`,
  `globe`), not emoji. The first paint comes from a cached `/api/menus`
  response (`NAV_CACHE_KEY` in `App.tsx`), never a build-time list.
- **Routes & pages**: **every business page is a UIPlugin** — core
  features live in `web/src/plugins/core/` (always composed, imported from
  `main.tsx` before the `#octarq-plugins` manifest module), Pro/third-party
  ones come from the manifest. All flow through the same registry
  (`registerUIPlugin` → `uiRoutes()`); the shell (`App.tsx`) owns
  only auth, settings, org handling, Overview and the plugin pipeline — never
  add a hardcoded business route there. `STATIC_AREAS` holds only **area/group
  shells** (label + order, items `[]`); a menu's `category` must equal its
  group label (`areaForCategory` places it), registration order is item order
  within a group, and empty groups/areas are dropped at runtime. The
  dynamic-fallback group is **"More"**, never "Plugin(s)". Plugins can declare
  new top-level areas via `UIPlugin.areas`; icons are string keys resolved by
  the single `PLUGIN_ICONS` table in `shell/areas.tsx`.

## Security invariants

- **Session invalidation on role change**: Changing a member's role (promote/demote)
  or removing a member must immediately invalidate all stateful sessions
  (`user_sessions`) for that user in that workspace (`org_id`), evicting them from
  cache. Sessions in other workspaces are unaffected. Audit log must record
  `actor`, `target`, `oldRole`, and `newRole`.


## Sandbox note

If a Vite build fails with "service was stopped", point `ESBUILD_BINARY_PATH` at the
real Mach-O binary under `node_modules/.pnpm/@esbuild+darwin-arm64@*/.../bin/esbuild`
(not the JS shim).
