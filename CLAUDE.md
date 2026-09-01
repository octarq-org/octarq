# Working in this repo (octarq)

Conventions for how changes are made here. Core open-source library and single-binary ops platform.

## Toolchain

- **Go 1.25**, standard-library `http.ServeMux`. Backend is pure-Go (no cgo).
- **Frontend (`web/`)**: Vite + React + TS + Tailwind. Package manager is **pnpm 9** (`packageManager: pnpm@9.15.4` in `web/`).
- **Never use npm.** Use pnpm.
- **Do NOT bump to pnpm 10/11 here.** They treat esbuild's build script as a fatal `ERR_PNPM_IGNORED_BUILDS` and break CI.

## Running & Dev Servers

- **Always start dev servers with `--host`**: `cd web && pnpm dev --host`.
- Backend: `OCTARQ_SECRET_KEY=dev OCTARQ_ADMIN_PASSWORD=dev go run .` (serves `:8080`).
- Full release build: `make release`.

## Verify before saying "done"

Run these and make sure they pass — don't claim a change works on inspection alone:
- `go build ./...`
- `go test ./... -race`
- `gofmt -w .`
- In `web/`: `npx tsc --noEmit`

## Embedded Dashboard (`webembed/dist`) — Critical Rule

- **Never build or commit `webembed/dist` manually.** It is tracked in git for downstream consumption (e.g. `octarq-pro`), but refreshed automatically by CI post-merge via a `chore(web): refresh embedded dashboard build` PR.
- On any branch touching `web/`, committed `webembed/dist` is stale until merged. Test frontend live via `cd web && pnpm dev --host` against the Go backend.

## Architecture & Code Conventions

- **Single source of truth — derive, don't duplicate**: Derive mappings dynamically (e.g., `areaForPath` in `web/src/shell/areas.tsx`). Collapse parallel hardcoded tables.
- **Sidebar & Routes**: Sidebar menus come strictly from the Go backend (`MenuProvider` / `/api/menus`). Frontend plugins register routes (`registerUIPlugin` → `uiRoutes()`) and UI components, never static menus.
- **Graceful degradation**: Optional/Pro feature pages must handle **402** (show upsell `LockedFeature`) and **404** (neutral note) gracefully.
- **Session invalidation on role change**: Changing a member's role or removing a member must immediately invalidate all stateful sessions (`user_sessions`) for that user in that workspace (`org_id`). Audit log must record `actor`, `target`, `oldRole`, and `newRole`.
- **Don't cram**: Split overgrown components into focused modules.

## Sandbox Note

If a Vite build fails with "service was stopped", point `ESBUILD_BINARY_PATH` at the real Mach-O binary under `node_modules/.pnpm/@esbuild+darwin-arm64@*/.../bin/esbuild` (not the JS shim).
