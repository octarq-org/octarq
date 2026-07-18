# Phase 3 — Composable core plugins (frontend self-containment + build-time exclusion + dependency contract)

Status: plan settled 2026-07-18, not started. Follow-up to `CORE-PLUGIN-EXTRACTION.md`
(phases 1+2 merged as PR #7/#8: backend links/mail/dns are `Core:true` plugins that
own their models, engines, and webhooks).

Three goals, in dependency order:

1. **Dependency contract** — plugins declare what they require (`mail → dns, links`;
   `links → dns`); the app refuses to start on an unsatisfiable composition.
2. **Build-time exclusion** — an edition can compile out a core feature (e.g. a
   links-only binary without mail) via build tags, and the excluded feature's code
   is actually absent from the binary, not just unmounted.
3. **Frontend self-containment** — each feature's UI (pages, components, API calls,
   i18n) lives inside its plugin directory and is composed through the same
   manifest pipeline Pro plugins use, so excluding a backend feature can exclude
   its frontend too.

## Current state (verified 2026-07-18)

**What already works:**
- Backend: `plugins/{dns,links,mail}` are self-contained (models + CRUD + engines +
  webhooks). Import graph is a DAG: `dns ← links ← mail`. Cross-plugin seams that
  must invert are already services: `dns.manager`, `mail.dispatcher`.
- Frontend: every business page is a `UIPlugin`; the SDK contract already supports
  `routes / menu / widgets / areas / i18n` per plugin; external plugins compose via
  the `#octarq-plugins` manifest virtual module (`web/plugins-manifest.ts`), and
  missing packages 404-degrade.

**What does NOT exist yet (the gaps this phase closes):**
- `plugin.Info` has no dependency field; `mail`'s need for `dns`/`links` is only a
  Go import + an implicit "their tables exist" assumption.
- The three core plugins are hardcoded in `app.New()` (`a.Use(dns.New())` …);
  there is no way to exclude one, and even unmounting would break core code.
- **Core still imports the plugins** — excluding a plugin from the build is
  impossible until these are decoupled (inventory, verified by grep):

  | Core file | Imports | Uses it for |
  |---|---|---|
  | `internal/api/overview.go` | dns, links, mail | dashboard counts/top-lists over feature tables |
  | `internal/api/account.go` | dns, links, mail | org purge deletes feature rows |
  | `internal/api/api.go` | links | (line ~25; small — inspect and relocate) |
  | `internal/api/helpers.go` | dns, mail | shared lookups |
  | `internal/api/abuse.go` | links | abuse report resolves a link |
  | `internal/api/ai.go` | mail | AI email-summarize reads `mail.Email` |
  | `internal/api/tenant_menu.go` | mail | menu/host bits |
  | `internal/mcp/tools.go` | dns, links, mail | MCP tools query feature tables |
  | `internal/cleanup/cleanup.go` | links | `LinkEvent` retention sweep |
  | `internal/db/db.go` | dns | legacy provider→ProviderAccount migration |
- Frontend feature code is scattered: plugin *defs* are in `web/src/plugins/core/`
  but pages live in `web/src/pages/{Links,Mail,Domains}.tsx` (+ `pages/{links,mail,domains}/`
  component dirs), API calls in the god-file `web/src/api.ts` (~865 lines), i18n in
  `web/src/i18n/pages/*.ts`. Core UI plugins are composed by a hardcoded
  `import "./plugins/core"` in `main.tsx`, outside the manifest.

## Design decisions

- **Dependency declaration**: add `Requires []string` to `plugin.Info` (plugin
  *names*). Validation = a new preflight in `app` (alongside
  `preflightTableCollisions`): every name in every mounted plugin's `Requires`
  must be present in the registered set, else refuse startup with a clear error
  naming both plugins. No auto-include, no soft-degrade — composition is an
  edition author's explicit choice, failures should be loud and at boot.
  Declare: `links.Requires = ["dns"]`, `mail.Requires = ["dns", "links"]`.
- **Exclusion mechanism**: Go build tags on *registration files*, not scattered
  through code. Create `plugins/builtin/` with one file per feature:

  ```go
  // plugins/builtin/links.go
  //go:build !octarq_nolinks
  package builtin
  import "github.com/octarq-org/octarq/plugins/links"
  func init() { register(links.New()) }
  ```

  plus `builtin.go` holding the `register`/`All()` slice. `app.New()` replaces the
  three hardcoded `a.Use(...)` with `for _, p := range builtin.All() { a.Use(p) }`.
  `go build -tags octarq_nomail` then drops mail — and once the decoupling below
  lands, the linker actually omits `plugins/mail`.
  Tag names: `octarq_nolinks`, `octarq_nomail`, `octarq_nodns` (note: `nodns`
  implies the other two via `Requires` — the preflight will say so at boot; CI
  only needs to prove `nomail` and `nomail+nolinks` shapes).
- **Decoupling pattern — one generic service per concern, not N bespoke ones.**
  Where core code needs "something from whichever feature plugins are present",
  the plugin `Provide`s a well-known service and core `Lookup`s all it needs,
  skipping absences:
  - `"<name>.overview"` → `func(orgID uint) map[string]any` — each plugin returns
    its own counts/top-lists; `overview.go` merges maps; absent plugin → absent
    keys (frontend already tolerates missing fields — verify, see WS-B).
  - `"<name>.purge"` → `func(orgID uint) error` — `account.go` iterates all
    registered purgers when deleting an org.
  - `"<name>.mcptools"` → contribution the MCP server iterates (mirror however
    `internal/mcp` registers tools today; keep its existing tool names/schemas
    byte-identical).
  - `"<name>.cleanup"` → `func(ctx context.Context)` retention sweep; `cleanup`
    runs whatever is provided (moves the `LinkEvent` sweep body into links).
  - One-off references (`abuse.go`, `ai.go`, `helpers.go`, `tenant_menu.go`,
    `api.go`): prefer **moving the route/handler into the owning plugin** over
    inventing a service — e.g. AI email-summarize belongs to mail (it can keep
    using `ctx.SetLLMResolver`-adjacent seams; check what it needs). Only add a
    service where the logic is genuinely core (abuse reports are core; give links
    a `"links.resolve"` service for slug→link lookup).
  - `internal/db/db.go`'s legacy dns migration: move into the dns plugin (run it
    from a `Starter`/first-mount hook before serving; it is idempotent — verify).
- **Frontend layout**: each feature becomes `web/src/plugins/<feature>/` containing
  `index.ts` (the UIPlugin def, incl. `i18n` and later `widgets`), `pages/`,
  `components/`, `api.ts` (its slice of the god-file; the shared fetch client and
  cross-cutting helpers stay in core `src/api.ts` or move to the SDK). i18n
  strings for the feature move from `src/i18n/pages/<feature>.ts` into the plugin
  via the `UIPlugin.i18n` field (the SDK already supports it; the "route-less
  i18n-only plugin" pattern in `plugins-manifest.ts` comments is precedent).
- **Frontend composition**: core feature plugins move from the hardcoded
  `import "./plugins/core"` into the **default manifest** as local-path entries,
  same pipeline as Pro. Truly-core UI (abuse, audit, assets/ProGate) stays in-tree
  and always composed. An edition that compiles out mail ships a manifest without
  the mail entry; if the frontend is present but the backend is absent, pages
  already 404-degrade (keep that behavior as the safety net).
- **Overview page**: backend merged-map means absent features must render as
  absent sections, not zeros/errors. Mid-term the right shape is
  `UIPlugin.widgets` contributions; in this phase just make `Overview.tsx`
  tolerate missing keys (only if it doesn't already).

## Work plan — 5 PRs, each independently green

Verify per repo convention before calling any PR done: `go build ./...`,
`go test ./... -race`, `gofmt -w`, `cd web && npx tsc --noEmit`. Never touch
`webembed/dist`. Frontend must keep 402/404 degradation conventions.

### PR-1: dependency contract (small, no behavior change)
1. Add `Requires []string` to `plugin.Info` (doc comment: names, validated at boot).
2. Declare `Requires` on links (`dns`) and mail (`dns`, `links`).
3. New preflight in `app` (both `RunMCP` and `Run` paths — remember there are TWO
   setup paths): validate every mounted plugin's `Requires` against the registered
   name set; error message must name the plugin, the missing dependency, and the
   build tag that likely caused it.
4. Tests: unit test the preflight (satisfied / missing / Core-missing cases).

### PR-2: decouple core → plugins (the big one; sequence within it, always green)
Work one consumer at a time; after each step the tree builds and tests pass:
1. `internal/db/db.go` legacy migration → dns plugin.
2. `overview.go` → `"<name>.overview"` services (dns/links/mail each provide;
   core merges). Migrate/adjust overview tests; add a test for "plugin absent →
   key absent, no error".
3. `account.go` purge → `"<name>.purge"` services. The purge order constraint
   (emails before mailboxes, events before links) moves with the code into each
   plugin's own purger.
4. `abuse.go` → `"links.resolve"` service.
5. `ai.go` email-summarize → move the route into the mail plugin (keep path/
   operation identical) or, if it is tangled with core AI-assist plumbing, a
   `"mail.email.get"` service — prefer the move.
6. `helpers.go`, `tenant_menu.go`, `api.go` residue → move or service per case.
7. `internal/cleanup` → `"<name>.cleanup"` services.
8. `internal/mcp/tools.go` → plugin-contributed MCP tools; tool names, schemas,
   and behavior must stay byte-identical (existing mcp tests are the guard).
9. Finish: `grep -rn "octarq/plugins/" internal/` must return **nothing**
   (test files may keep imports only if they test plugin-owned behavior — prefer
   moving such tests into the plugin packages).
Route/OpenAPI parity check (same trick as phases 1/2): extract
`huma.Operation{...}` blocks before/after and diff — must be identical.

### PR-3: build-tag composition
1. `plugins/builtin/` package as designed above; `app.New()` iterates
   `builtin.All()`.
2. CI matrix additions: `go build ./...` for the default shape plus
   `-tags octarq_nomail` and `-tags "octarq_nomail octarq_nolinks"`; boot each
   excluded shape far enough to prove startup succeeds and `/api/mailboxes`
   returns 404 (a smoke test in `app` using the in-memory DB is enough — no
   Docker, rely on CI).
3. Verify with `go tool nm` (or binary-size diff) that an excluded plugin's
   symbols are gone — this is the acceptance test for PR-2's decoupling.

### PR-4: frontend feature self-containment (mechanical move, no manifest change yet)
1. `web/src/plugins/links/`: move `pages/Links.tsx` → `pages/index.tsx` (or
   keep names), `pages/links/*` components, the links slice of `src/api.ts`,
   `i18n/pages/links.ts` (wired via `UIPlugin.i18n`). Update the plugin def's
   lazy imports. Same for mail (incl. `pages/mail/types.ts`) and domains/dns.
2. Shared leftovers: `effectiveLinkHosts`/`effectiveMailHosts` and any
   cross-feature types — keep in core `api.ts` only if ≥2 consumers remain,
   else move with their feature. **Do not reintroduce duplicated path→area or
   host-derivation logic** (CLAUDE.md single-source rule).
3. `src/api.ts` shrinks to: fetch client, auth/org/settings/overview calls.
4. `plugins/core/index.ts` now imports from the new locations; `tsc` and the
   Vite build stay green; no user-visible change.

### PR-5: manifest-composed core UI
1. Move links/mail/domains entries from the hardcoded `plugins/core` import into
   the default manifest (local-path entries, ordered so core menu items precede
   Pro within shared groups — registration order is menu order).
2. Keep abuse/audit/assets in-tree always-composed.
3. Prove exclusion end-to-end: build a manifest without mail + backend
   `-tags octarq_nomail`; the mail menu/pages are absent, nothing 404s from the
   sidebar, Overview shows no mail section.
4. Document the edition recipe (tags + manifest) in this file's tail or README.

## Gotchas carried over from phases 1/2 (still apply)
- TWO `plugin.Context`/setup paths in `app/app.go` (MCP + Run) — every wiring
  change lands in both. Phase 1 shipped a real bug by missing one; the harness
  masks it, so also add/keep a test that mounts via the *production* wiring.
- `preflightTableCollisions` semantics: plugins are now sole owners of their
  tables; keep it that way.
- pnpm 9 only; never commit `webembed/dist`; push and let CI verify rather than
  local Docker.
- octarq-pro consumes octarq as a Go module and the SDK from npm — `plugin.Info`
  gains a field (additive, fine) and `UIPlugin` is unchanged; do not break or
  rename existing `Context` fields or services (`dns.manager`, `mail.dispatcher`).

## Custom Edition Build Recipes

Core feature plugins (`dns`, `mail`, `links`) can be selectively excluded at compile time from both the backend Go binary and the frontend web UI to produce lightweight, custom-tailored editions of octarq.

### Build Tags (Go Backend)
Use Go build tags during `go build` to omit unwanted feature plugins:
- `-tags octarq_nomail`: Exclude the `mail` plugin.
- `-tags octarq_nolinks`: Exclude the `links` plugin.
- `-tags octarq_nodns`: Exclude the `dns` plugin (Note: `nodns` also implies `nomail` and `nolinks` due to dependency preflight).

Example:
```bash
go build -tags "octarq_nomail octarq_nolinks" -o octarq-slim ./cmd/octarq
```

### Frontend Manifest Composition (Vite / React Frontend)
Control which UI plugins are bundled by configuring `web/octarq.plugins.json` or setting `OCTARQ_PLUGINS` / `OCTARQ_PLUGINS_MANIFEST` environment variables during `npm run build`:

Example (`octarq.nomail.json`):
```json
{
  "plugins": [
    "./src/plugins/dns",
    "./src/plugins/links"
  ]
}
```

Build command:
```bash
OCTARQ_PLUGINS_MANIFEST=octarq.nomail.json npm run build
```

