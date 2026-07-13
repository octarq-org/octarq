# Contributing to octarq

Thanks for helping improve octarq. This covers core contributions; if you want to
build a **feature as a plugin** (the way Pro features and third-party extensions
are built, with no fork), see the **[plugin development guide](docs/PLUGINS.md)**.

## Toolchain

- **Go 1.25** (standard-library `http.ServeMux`, pure-Go, no cgo).
- **Frontend in `web/`**: Vite + React + TypeScript + Tailwind 4. Package manager
  is **pnpm 9** (`packageManager: pnpm@9.15.4`). **Never use npm.** Do **not**
  bump pnpm to 10/11 (it breaks CI's esbuild build-script handling).

## Running it

- Backend: `OCTARQ_SECRET_KEY=dev OCTARQ_ADMIN_PASSWORD=dev go run .` (serves `:8080`).
- Frontend dev: `cd web && pnpm dev --host`.
- Full build: `make release`.

## Before you open a PR

Run these and make sure they pass — don't claim a change works on inspection alone:

- `go build ./...`
- `go test ./... -race`
- `gofmt -w` (keep gofmt-clean; match surrounding style)
- in `web/`: `npx tsc --noEmit`
- if you touched `packages/plugin-sdk/`: `pnpm --filter @octarq-org/plugin-sdk test`

CI runs the same, plus builds the dashboard. Prefer pushing and relying on CI
over spinning up Docker locally.

## Conventions

- **Single source of truth — derive, don't duplicate.** Collapse parallel
  hardcoded mappings when you find them.
- **Don't cram.** Split overgrown components/config into focused modules.
- **Every business page is a UIPlugin.** The shell (`web/src/App.tsx`) owns only
  auth, settings, org handling, Overview and the plugin pipeline; core features
  live in `web/src/plugins/core/` and register through the same
  `registerUIPlugin` registry as Pro/third-party plugins. Never add a hardcoded
  business route to the shell.
- **Optional/Pro features degrade gracefully.** Every plugin route is wrapped in
  `ProGate`: **402** → upsell, **403** → access denied, **404** / chunk failure →
  a neutral "not in this build" note — never a raw error. Pages may still handle
  these themselves; the gate is the safety net.
- **Shared UI** lives in `web/src/ui` (shadcn / Base UI backed) and is re-exported
  to plugins via `@octarq-org/plugin-sdk`. Build UI from those primitives.
- **Security-sensitive changes** (auth, crypto, tenant isolation, SSRF) must come
  with tests; see `internal/api/isolation_test.go` and `internal/api/csrf_test.go`
  for the patterns.

## Commits & PRs

- Write focused commits with clear messages; keep unrelated changes separate.
- Reference issues where relevant. A green CI is required to merge.
