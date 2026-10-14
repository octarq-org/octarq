# SPEC: `internal/server`

## Purpose

Top-level HTTP host composition: API mux, embedded SPA, dev proxy, static
plugin mounts (buyer portal etc.), edge middleware dispatch (rate limit,
metrics, request IDs, logging), and host-based dashboard gating.

## Routing

- `/api/*` and `/auth/*` → JSON API
- spec paths (`/openapi.json`, `/docs`, `/schemas/...`) → API reference
- `/admin/*`, `/status`, `/instance/*`, `/signup`, `/login`, `/robots.txt`
- Plugin static mounts registered via `plugin.Context.HandleStatic`
- Everything else in the root namespace → `rootFallback` (if any) else 404

## Key Types

- `StaticMount` — plugin-contributed SPA asset filesystem
- `Server` — combined handler
- `RuntimeSettings` — DB-backed runtime knobs for the edge middleware
  (`metrics token`, rate-limit thresholds)

## Composition

`New(cfg, db, apiHandler, rootFallback, webFS, mounts, rs)` builds the
combined handler. `webFS` is required in production (release) build; in dev
proxy mode a read failure is logged (no silent `_ =`).

## Concurrency

`ServeHTTP` delegates to `middleware.handle` which runs the edge middleware
chain then dispatches via `Server.route`. No shared mutable server state after
construction.

## Error Handling

- JSON body write failures in `writeRateLimited` and `serveMetrics` are now
  logged via `slog.Warn` instead of ignored.
- `fs.ReadFile` failure during dev-proxy mode is logged.
- Remaining `_ =` site: writing `/robots.txt` body (client disconnect) —
  documented benign.

## Non-Goals

- No user-facing help docs (internal package).
- No direct dependency on `internal/api` (avoids import cycle).
EOF
echo done