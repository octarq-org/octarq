# Contributing to led

Thanks for helping improve led. This covers core contributions; if you want to
build a **feature as a plugin** (the way Pro features and third-party extensions
are built, with no fork), see the **[plugin development guide](docs/PLUGINS.md)**.

## Toolchain

- **Go 1.25** (standard-library `http.ServeMux`, pure-Go, no cgo).
- **Frontend in `web/`**: Vite + React + TypeScript + Tailwind 4. Package manager
  is **pnpm 9** (`packageManager: pnpm@9.15.4`). **Never use npm.** Do **not**
  bump pnpm to 10/11 (it breaks CI's esbuild build-script handling).

## Running it

- Backend: `LED_SECRET_KEY=dev LED_ADMIN_PASSWORD=dev go run .` (serves `:8080`).
- Frontend dev: `cd web && pnpm dev --host`.
- Full build: `make release`.

## Before you open a PR

Run these and make sure they pass — don't claim a change works on inspection alone:

- `go build ./...`
- `go test ./... -race`
- `gofmt -w` (keep gofmt-clean; match surrounding style)
- in `web/`: `npx tsc --noEmit`

CI runs the same, plus builds the dashboard. Prefer pushing and relying on CI
over spinning up Docker locally.

## Conventions

- **Single source of truth — derive, don't duplicate.** Collapse parallel
  hardcoded mappings when you find them.
- **Don't cram.** Split overgrown components/config into focused modules.
- **Optional/Pro features degrade gracefully.** A page hitting a plugin endpoint
  handles **402** (upsell) and **404** (plugin absent in this build) — never a
  raw error.
- **Shared UI** lives in `web/src/ui` (shadcn / Base UI backed) and is re-exported
  to plugins via `@led/plugin-sdk`. Build UI from those primitives.
- **Security-sensitive changes** (auth, crypto, tenant isolation, SSRF) must come
  with tests; see `internal/api/isolation_test.go` and `internal/api/csrf_test.go`
  for the patterns.

## Commits & PRs

- Write focused commits with clear messages; keep unrelated changes separate.
- Reference issues where relevant. A green CI is required to merge.
