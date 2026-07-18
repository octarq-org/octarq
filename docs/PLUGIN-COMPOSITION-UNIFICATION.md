# Phase 4 — Unify Core plugin composition with the Pro model (drop build tags)

Status: plan settled 2026-07-18, not started. Follow-up to `PLUGIN-COMPOSABILITY.md`
(Phase 3, merged as PR #9). Cross-repo: touches **octarq** and **octarq-pro**.

## The problem this fixes

Phase 3 left two different mental models for "which plugins ship in a build":

- **Pro plugins & the whole frontend are opt-in** — a *composition root* explicitly
  lists them. `octarq-pro`'s `cmd/octarq-pro/main.go` does `a.Use(proPlugin)`; the
  frontend lists every UI plugin (core feature ones included) in
  `web/octarq.plugins.json`. Excluding one = remove it from the list.
- **Backend Core plugins are opt-out via Go build tags** — `app.New()` auto-mounts
  them through `builtin.All()`, and you *subtract* one with
  `go build -tags octarq_nomail`. `octarq_no*` is "on by default, tag turns it off".

So the frontend already excludes `mail` by editing a manifest while the backend
excludes it by recompiling with a tag. Two composition mechanisms, opposite
polarity (opt-out vs opt-in). This phase makes the backend match the Pro/frontend
opt-in model and **removes build tags entirely**.

## The key realization

Go's linker already does dead-code elimination: **if no reachable code imports
`plugins/mail`, its package never enters the binary.** "Compile-time exclusion"
therefore needs only *a composition root that doesn't reference the plugin* — not a
build tag. Build tags were only necessary because a *single* `main` wanted to be
both the full build and the trimmed build. Once each edition owns its composition
root (which Pro already does), tags are redundant.

## Current state (verified 2026-07-18)

- `octarq/app/app.go` `New()` ends with `for _, p := range builtin.All() { a.Use(p) }`
  — Core plugins are auto-mounted inside the constructor.
- `octarq/plugins/builtin/` = `builtin.go` (`All()`) + `{dns,mail,links}.go`
  (`//go:build !octarq_no<x>` → real `New()`) + `{dns,mail,links}_stub.go`
  (`//go:build octarq_no<x>` → returns nil). Six tag-split files.
- `octarq/main.go` (OSS entry): `a, _ := app.New(); a.Run(ctx)`. Also has an `mcp`
  subcommand that calls `internal/mcp.Run(ctx)` **(NOT via app — see Gotchas)** and
  an `openapi` subcommand calling `openapi.Generate(os.Stdout, nil)`.
- `octarq-pro/cmd/octarq-pro/main.go`: `a, _ := app.New()` (inherits Core plugins
  from `builtin.All()` implicitly), then `a.Use(platform.NewEntitlementGate)` +
  `for p := range selectPlugins(lic) { a.Use(p) }`. Its `openapi` path passes
  `proPlugins(nil)`; its `mcp` path calls `a.RunMCP(ctx)`.
- App tag tests: `app/{nomail,nolinks,nodns}_test.go` (`//go:build octarq_no<x>`) +
  `app/build_tags_test.go`. CI builds `-tags octarq_nomail` and
  `-tags "octarq_nomail octarq_nolinks"`.
- `plugin.Info.Requires` + `app.preflightDependencies` already exist (Phase 3) and
  become *more* load-bearing under opt-in (forgetting to `Use(dns)` while using
  `mail` must fail loudly at boot — it already does).

## Design

**Opt-in everywhere. `app.New()` mounts nothing by default; the composition root
decides.**

1. **`builtin` becomes a plain default set, no tags.**
   - Delete the six tag-split files. Replace with a single `builtin.go`:
     ```go
     package builtin
     import (
         "github.com/octarq-org/octarq/plugin"
         "github.com/octarq-org/octarq/plugins/dns"
         "github.com/octarq-org/octarq/plugins/links"
         "github.com/octarq-org/octarq/plugins/mail"
     )
     // Default is the OSS edition's Core feature set, in dependency order
     // (dns before links before mail). A trimmed edition builds its own
     // composition root that Uses a subset; unreferenced plugin packages are
     // dropped from the binary by the linker (no build tags needed).
     func Default() []plugin.Plugin {
         return []plugin.Plugin{dns.New(), links.New(), mail.New()}
     }
     ```
   - Rename `All()` → `Default()` to signal "the default set", not "everything that
     survived tags".
2. **`app.New()` stops auto-mounting.** Remove the `builtin.All()` loop from `New()`.
   `New()` returns an app with **zero** feature plugins; the caller composes.
   - Keep `app.Use()` exactly as is (the composition API, shared by core & Pro).
3. **OSS `main.go` composes explicitly.** After `app.New()`:
   ```go
   for _, p := range builtin.Default() { a.Use(p) }
   ```
   This is the OSS edition's composition root — the exact analog of
   `web/octarq.plugins.json` listing the three feature plugins.
4. **octarq-pro `main.go` composes Core the same way.** Insert, before the Pro
   `Use` loop:
   ```go
   for _, p := range builtin.Default() { a.Use(p) }
   ```
   Now Core and Pro plugins are mounted by the **same** `a.Use()` mechanism in the
   Pro composition root — the unification the user asked for. (Pro may instead want
   to select a Core subset per product recipe later; `Default()` is the drop-in
   that preserves today's behavior.)
5. **Trimmed edition = a composition root that Uses a subset.** Document the recipe:
   write a `main` (or reuse one via a small helper) that Uses only the wanted
   plugins; do not import the others. Provide a **worked example + CI proof** (see
   PR-2) so the mechanism is demonstrated and regression-guarded.
6. **Delete all build tags.** Remove `octarq_no{dns,mail,links}` from everywhere:
   the stub files, the app tag-tests, and the CI matrix rows.

## Frontend

Already opt-in via the manifest (Phase 3). **No structural change.** Confirm only:
the committed default `web/octarq.plugins.json` lists dns/mail/links (it does), and
the "trim a feature" recipe (drop a manifest line, optionally 404-degrade) is
documented alongside the backend recipe so both halves of an edition are described
in one place.

## Work plan — 3 PRs

Verify per repo convention before calling any PR done: `go build ./...`,
`go test ./... -race`, `gofmt -w`, and in `web/` `tsc --noEmit`. Never touch
`webembed/dist`.

### PR-1 (octarq): de-tag `builtin`, move composition to the entry points
1. Rewrite `plugins/builtin/` to the single tag-free `Default()` above; delete the
   six tag-split files.
2. Remove the `builtin.All()` loop from `app.New()`.
3. Add the `builtin.Default()` `Use` loop to `octarq/main.go` (both the normal Run
   path AND the `mcp`/`openapi` subcommand paths if they build an app — **see
   Gotchas**, the OSS `mcp` path currently does NOT go through the app).
4. Replace the tag-based app tests: delete `app/{nomail,nolinks,nodns}_test.go` and
   the `octarq_no*` parts of `build_tags_test.go`; add a **composition test** that
   builds an `App`, `Use`s only `dns.New()`+`links.New()` (no mail), runs the same
   preflight/route assertions, and asserts mail routes are absent and
   `preflightDependencies` still passes (links→dns satisfied). Add a negative test:
   `Use(mail.New())` without `links`/`dns` → `preflightDependencies` errors.
5. Update CI: drop the `-tags octarq_nomail` / `-tags "octarq_nomail octarq_nolinks"`
   build rows. Replace with PR-2's example-build job (or a placeholder until PR-2).
6. Update `docs/PLUGIN-COMPOSABILITY.md` (its Phase-3 "build tag" section is now
   historical) and this file's status.

### PR-2 (octarq): worked trimmed-edition example + linker-exclusion proof
1. Add a minimal example composition root, e.g. `examples/edition-linksonly/main.go`
   (or `cmd/octarq-linksonly/`), that `Use`s only `dns.New()`+`links.New()`.
   (dns is required by links per `Requires`; a links-only build still needs dns
   mounted — the example doubles as `Requires` documentation.)
2. Exclusion proof as a Go test (`examples/edition-nomail/exclude_test.go`): it
   `go build`s the example and asserts via `go tool nm` that `plugins/mail`
   symbols are **absent** — the opt-in analog of Phase 3's nm check, proving DCE
   works without tags. Runs under the normal `go test ./...` so no CI workflow
   edit is needed (the push token lacks `workflow` scope anyway).
3. Document the full edition recipe (backend composition root + frontend manifest)
   in `docs/PLUGIN-COMPOSABILITY.md` or a new `docs/EDITIONS.md`.

### PR-3 (octarq-pro): compose Core explicitly
1. In `cmd/octarq-pro/main.go`, add `for _, p := range builtin.Default() { a.Use(p) }`
   before the Pro `Use` loop (import `octarq/plugins/builtin`). Behavior is
   identical to today (Pro already got all three via `builtin.All()` inside
   `New()`), but now the composition is explicit and uniform with the Pro plugins.
2. Fix the `openapi` and `mcp` paths analogously if they construct plugin sets
   independently (`proPlugins(nil)` / `a.RunMCP`) — Core plugins must be present
   for OpenAPI/route generation and MCP tools. Verify the generated OpenAPI is
   byte-identical to pre-change (same trick: diff the operation set).
3. Pin the octarq module version to the PR-1 commit; run pro's full build/test.
   Land PR-3 only after PR-1 is merged so `app.New()` no longer auto-mounts (else
   Pro would double-mount → `preflightTableCollisions`/duplicate-route surprise).

## Sequencing & compatibility (important)

`app.New()` dropping auto-mount is a **breaking change for any external consumer of
the octarq module** (octarq-pro is the known one). Order:
1. Merge PR-1 (octarq) — OSS `main.go` updated in the same PR so the OSS binary
   stays whole. **The moment PR-1 lands, an un-updated octarq-pro build loses its
   Core plugins.** So:
2. Merge PR-3 (octarq-pro) immediately after, bumping to PR-1's octarq version, in
   lockstep. Do not leave `main` of pro on an old octarq across releases.
3. PR-2 (example + CI proof) can land between or after; it's additive.

If a smoother migration is wanted, an interim `app.NewWithDefaults()` =
`New()` + `Use(builtin.Default()...)` could be added in PR-1 and pro switched to it,
then removed later — but given pro is the only consumer and both repos are yours,
the lockstep two-PR swap is simpler. Recommend lockstep.

## Gotchas

- **OSS `mcp` subcommand bypasses the app.** `octarq/main.go`'s `mcp` path calls
  `internal/mcp.Run(ctx)` directly, not `app.New()`. Check how it obtains the
  plugin set for MCP tools today (Phase 3 made MCP tools plugin-provided). If it
  builds its own plugin list, it needs the same `builtin.Default()` composition;
  if it reuses app wiring, route it through the composed app. **Don't let the OSS
  `mcp` command silently lose links/mail/dns tools.**
- **`openapi` generation must include Core routes.** Both entries generate OpenAPI
  (`openapi.Generate(nil)` OSS; `proPlugins(nil)` pro). Under opt-in, the generator
  must be handed the composed plugin set or its routes vanish from the spec. Verify
  the generated spec is unchanged.
- **Two Context/setup paths in `app/app.go`** (RunMCP + Run) — unchanged by this
  phase (composition happens before either), but the removal of the `builtin.All()`
  loop is in `New()`, singular. Confirm nothing else in `New()` assumed plugins
  were present.
- **`preflightDependencies` is now the safety net.** Under opt-in, a composition
  that Uses `mail` but forgets `dns`/`links` must fail at boot. It already does;
  keep the negative test (PR-1 step 4) so this never regresses.
- **Don't reintroduce a parallel registry.** `builtin.Default()` is the single
  source for "the OSS default Core set"; the frontend manifest is its UI analog.
  Keep them the only two lists; don't hardcode the trio anywhere else.
- pnpm 9 only; never commit `webembed/dist`; push and let CI verify.

## Outcome

One composition model across the stack: every plugin — Core or Pro, backend or
frontend — is mounted by an explicit `a.Use()` / manifest entry in a composition
root. No build tags, no `octarq_no*` polarity, no tag-combination matrix. Trimming
an edition means writing a composition root that omits a plugin; the linker drops
the code.
