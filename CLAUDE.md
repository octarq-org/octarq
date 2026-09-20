# Working in this repo (octarq)

Conventions for how changes are made here. Core open-source library and single-binary ops platform.

## Toolchain

- **Go 1.25**, standard-library `http.ServeMux`. Backend is pure-Go (no cgo).
- **Frontend (`web/`)**: Vite + React + TS + Tailwind. Package manager is **pnpm 11** (`packageManager: pnpm@11.8.0` in `web/`).
- **Never use npm.** Use pnpm.
- pnpm 10+ treats esbuild's build script as a fatal `ERR_PNPM_IGNORED_BUILDS` unless approved: `web/pnpm-workspace.yaml` sets `allowBuilds: {esbuild: true}` + `onlyBuiltDependencies: [esbuild]` (and `verifyDepsBeforeRun: false`, mirroring octarq-pro). Keep that list minimal if new postinstall deps appear.

## Running & Dev Servers

- **Always start dev servers with `--host`**: `cd web && pnpm dev --host`.
- Backend: `cd server && OCTARQ_SECRET_KEY=dev OCTARQ_ADMIN_PASSWORD=dev go run .` (serves `:8080`).
- From repo root: `make dev` runs both backend (Air hot reload) and frontend (Vite).
- Full release build: `make release` (or `make build`).

## Verify before saying "done"

Run these and make sure they pass — don't claim a change works on inspection alone:
- From root: `make build` (or in `server/`: `go build ./...`)
- In `server/`: `go test ./... -race`
- In `server/`: `gofmt -w .`
- In `web/`: `pnpm typecheck` and `pnpm test`

## Change Workflow (worktree + pr-ship)

- **All new changes go through a git worktree** (under `/Volumes/PHD/code/.worktrees/`, one branch per change). Never develop directly on a `main` checkout.
- **After work completes, automatically run `/pr-ship`**: open PRs, gate on real CI green, squash-merge (OSS first, then Pro bump, then Pro), and clean up branches/worktrees.
- Coverage gates stay as-is; never lower thresholds to make CI pass.

## Embedded Dashboard (`server/webembed/dist`) — Critical Rule

- **Never build or commit `server/webembed/dist` manually.** It is tracked in git for downstream consumption (e.g. `octarq-pro`), but refreshed automatically by CI post-merge with a direct `chore(web): refresh embedded dashboard build [auto]` commit to `main`.
- On any branch touching `web/`, committed `server/webembed/dist` is stale until merged. Test frontend live via `cd web && pnpm dev --host` against the Go backend.

## Architecture & Code Conventions

- **Three-Tier Documentation Model (Scheme B)**:
  - **Tier 1 (User Help Docs)**: Embedded in `server/plugins/*/docs/` via `plugin.HelpDocsFS` (`//go:embed docs`). Must have bilingual `.zh.mdx` translations, strictly covering user-facing workflows and UI usage.
  - **Tier 2 (Module Living Spec)**: `server/plugins/<name>/SPEC.md` co-located in each plugin. **Strictly NOT embedded in binary**. Serves as the technical truth for state machines, internal event flows, database models, and error states.
  - **Tier 3 (Executable Contracts)**: Pure Go interfaces, route-derived OpenAPI specs, and contract tests (`*_test.go`).
- **Single source of truth — derive, don't duplicate**: Derive mappings dynamically (e.g., `areaForPath` in `web/src/shell/areas.tsx`). Collapse parallel hardcoded tables.
- **Sidebar & Routes**: Sidebar menus come strictly from the Go backend (`MenuProvider` / `/api/menus`). Frontend plugins register routes (`registerUIPlugin` → `uiRoutes()`) and UI components, never static menus.
- **Graceful degradation**: Optional/Pro feature pages must handle **402** (show upsell `LockedFeature`) and **404** (neutral note) gracefully.
- **Pre-v1.0 Stage (未发布 v1.0 铁律)**:
  - **Engineering Invariants**: All changes must adhere to the [Five Invariants & Trinity Law](docs/INVARIANTS.md).
  - **Trinity Interface Law**: Every feature must provide Web UI (`plugin-sdk`), REST API (OpenAPI), and Agent Tool (MCP with `org_id` isolation).
  - **严禁保留 fallback、软降级或双轨制兼容代码**。三方生态尚未建立，三方插件必须升级。
  - 不需要保留 deprecated 标记的旧接口或旧字段，直接彻底清理。
  - 核心全部通过 `plugin.Host` SPI 暴露，强制 `TenantDB` 隔离，不提供绕过租户的 raw DB fallback。
  - 缺失凭据或校验失败时**严格 Fail-Closed**，拒绝启动或报错。
  - **Redis is optional**: SQLite-only mode must be complete and pass all tests. Redis is for `asynq` enhancement only.
- **Don't cram**: Split overgrown components into focused modules.

## Sandbox Note

If a Vite build fails with "service was stopped", point `ESBUILD_BINARY_PATH` at the real Mach-O binary under `node_modules/.pnpm/@esbuild+darwin-arm64@*/.../bin/esbuild` (not the JS shim).
