# Extracting links / mail / dns into self-contained Core plugins

Status: design settled, implementation in progress on `refactor/core-feature-plugins`.
Goal: lift the three built-in features out of the monolithic `internal/api.Handler`
god-object into three **`Core: true` plugins** that mount by default — the same plugin
contract Pro features already use — with **no behavioural change** and the full test
suite green.

## Two-phase plan (decided 2026-07-17)

Full self-containment (plugins own their GORM models) requires an atomic move of
`Domain`/`Link`/`Mailbox` out of `internal/models` that breaks `internal/{shortlink,
mcp,db}` and much of the test suite at once. To land value in one session without a
broken tree, split it:

- **Phase 1 (this PR) — seam-preserving.** The three features become `Core:true`
  plugins that own their **routes + handler logic + the seam logic** (DNSManager,
  redirect engine, OnEmail dispatch), wired through the service registry. The model
  **types stay in `internal/models`** and are imported by the plugins. Consequences:
  the HTTP-level tests keep compiling (routes still registered, `models.X` still
  exists); `overview.go` counts stay untouched; core migration (`db.go`) is unchanged.
- **Phase 2 (follow-up) — full self-containment.** Physically relocate the model
  type declarations into their owning plugin packages, move migration to each
  plugin's `Models()`, and repoint the importers (`shortlink`/`mcp`/`db`). Phase 1
  already encapsulates the logic, so this shrinks to a contained type-move.

The ownership, seams, and services below are the Phase-1 target; Phase 2 only changes
*where the model structs are declared*, not the plugin boundaries.

## Why this is invasive (the findings that shape the design)

1. **One god-Handler.** All ~40 feature routes are methods on a single
   `api.Handler` sharing `db / auth / geo / queue / cipher / audit`, registered
   centrally in `api.go:Routes()`. Extraction = pulling routes, handler methods,
   and their private helpers onto per-plugin structs fed from `plugin.Context`.

2. **`Domain` is a shared aggregate, not a per-feature model.** A single
   `models.Domain` row carries **link hosts, mail hosts, and DNS/zone config**
   together (`normalizeHosts → models.HostList`, consumed by links *and* mail).
   So "each plugin owns its models" cannot mean "links owns nothing about
   domains". Resolution: **the dns plugin owns `Domain`** and exposes host
   resolution + DNS management as services; links/mail own only their leaf models.

3. **Three core-owned seams other code depends on must invert:**
   - `ctx.DNS` (`plugin.DNSManager`) — today `apiHandler.DNSManager()`, consumed by
     Pro's infra/ai. Must become a lazy delegator resolving the dns plugin's
     provided `"dns.manager"` service.
   - Root **redirect engine** (`/{slug}`) — core-served, reads `Link`. Moves into
     the links plugin via a new root-path registration hook on `Context`.
   - Inbound-email webhook + **`OnEmail`** dispatch — core-served (`emitEmail`),
     read `Mailbox`/`Email`; Pro's ai subscribes via `ctx.OnEmail`. The mail plugin
     takes over the webhook + dispatch; `ctx.OnEmail` registrations route to it.

4. **Inter-plugin coupling** (wire via `ctx.Provide`/`Lookup`, resolved lazily in
   `Start`/per-request):
   - `mail → links`  : `wrapLinksInEmail` (rewrite links in outbound mail).
   - `dns  → links`  : `checkLinkHost` (domain verify inspects link host status).
   - shared `isReservedSlug` / `isReservedMailbox` (settings-derived).

## Plugin ownership

| Plugin | Owns (models) | Serves | Provides (services) | Consumes |
|--------|---------------|--------|---------------------|----------|
| `dns`  | `Domain`, `ProviderAccount`, DNS record wire types | `/api/domains`, `/api/provider-accounts`, `/api/dns/providers`, `/api/domains/{id}/records…` | `dns.manager` (`plugin.DNSManager`), `domain.hosts` (link/mail host + verify lookup) | `links.hostcheck` |
| `links`| `Link`, `LinkEvent` | `/api/links…` + **root `/{slug}` redirect** | `links.wrap` (rewrite links in HTML), `links.hostcheck` | `domain.hosts` |
| `mail` | `Mailbox`, `Email`, `Attachment`, `SMTPSender` | `/api/mailboxes…`, `/api/emails…`, `/api/smtp-senders…`, inbound webhook | — (fires `OnEmail`) | `domain.hosts`, `links.wrap` |

## `plugin.Context` additions (OSS contract — additive, consumed by Pro)

Only stable/external types (no `internal/*` leakage), mirroring the existing style:

- `Queue Enqueue(kind string, payload any) error` — links click analytics / crawl
  (wraps `internal/queue`, no type leak).
- `GeoLookup func(ip string) (country, region, city string)` — link click geo
  (wraps `internal/geo`).
- `Cache` accessor — the resolver cache links uses (`h.auth.Cache`).
- `HandleRoot(pattern string, h http.Handler)` — register a **non-`/api`** root
  route (the redirect engine). Gated identically to plugin API routes.
- `Config` read accessors already covered by `Get/SetWorkspaceSetting`; mail DKIM/
  SPF/DMARC/Host come from settings, read through those.
- `ReservedSlug(slug string) bool` / equivalent, or expose via a core service.

`ctx.DNS` stops being built from the Handler; app wires it to a lazy delegator over
`Lookup("dns.manager")` so consumers are unchanged.

## Always-green step sequence (one PR, incremental compiling commits)

1. **Context extension** — add the accessors above, wire them from the Handler
   (still the source of truth). Nothing consumes them yet; tree stays green.
2. **Invert `ctx.DNS`** to the lazy `Lookup("dns.manager")` delegator, with the
   Handler still providing it. Green, behaviour identical.
3. **dns plugin** — move `Domain`/`ProviderAccount` + provider helpers +
   `domains.go`/`dns_manager.go` handlers into `plugins/dns`; delete from
   `internal/{api,models}`; register routes in its `Mount`; provide `dns.manager`
   + `domain.hosts`; fix `overview.go` counts via the service; migrate
   `domains_test`/`dns_manager_test`/`invite_dns_test`. Green.
4. **mail plugin** — `Mailbox`/`Email`/`Attachment`/`SMTPSender` + `mail.go`/
   `smtp.go` + inbound webhook + `OnEmail` dispatch into `plugins/mail`; consume
   `domain.hosts` + `links.wrap`; migrate mail tests. Green.
5. **links plugin** — `Link`/`LinkEvent` + `links.go` + **redirect engine** into
   `plugins/links` via `HandleRoot`; provide `links.wrap`/`links.hostcheck`;
   migrate link/redirect tests. Green.
6. **Cleanup** — shrink `api.Handler` to core-only (auth/org/settings/overview/
   notifications/tokens/abuse/audit/ai-assist); final `go build ./... && go test
   ./... -race`; open the single PR.

Each numbered step compiles and passes tests before the next begins.

---

# HANDOFF — execution status & step-by-step for the next implementer

This section is the working handoff. It reflects what is **already done on the
branch** `refactor/core-feature-plugins` and exactly what remains, precise enough
to execute without re-deriving anything.

## Refined phase-1 scope (important — read first)

The runtime engines can **stay in `internal/` for phase 1**; only the dashboard
CRUD/management moves into the plugins. Rationale: they read `models.*` directly
from the DB, and in phase 1 the model types stay in `internal/models`, so they
keep working untouched — moving them buys nothing in phase 1 and adds risk.

- **dns**: exception — its runtime seam (`plugin.DNSManager`) was small and
  cleanly invertible, so it **was** moved into the plugin and provided as the
  `dns.manager` service. (Done.)
- **mail**: the **inbound-email webhook + `emitEmail`/`OnEmail` dispatch STAY in
  `internal/api`** for phase 1. Only the mailbox/email/smtp-sender **dashboard
  CRUD routes move** into `plugins/mail`. This avoids inverting the `OnEmail`
  seam (Pro `ai` subscribes to it) — defer that to phase 2.
- **links**: the **root `/{slug}` redirect engine (`internal/shortlink`) + click
  recording STAY in core** for phase 1 (they read `models.Link`/`Domain`). Only
  the `/api/links*` **dashboard CRUD moves** into `plugins/links`. No
  `HandleRoot` Context addition needed in phase 1.

Net effect: **no `plugin.Context` additions are needed for mail/links CRUD** —
they need only `DB`, `OrgID`, `Audit`, `Encrypt`/`Decrypt`, and (mail) a couple of
small helpers replicated locally. The `Queue`/`GeoLookup`/`Cache`/`HandleRoot`
additions listed above are **phase-2** concerns (they move with the runtime
engines). This makes mail/links mechanically identical to what was done for dns.

## The mechanical conversion recipe (proven on dns — follow verbatim)

For each handler method moved from `func (h *Handler) X(...)` to
`func (p *Plugin) X(...)`:

1. `h.` → `p.` throughout.
2. Replace the auth preamble
   ```go
   r, _ := humago.Unwrap(input.Ctx)
   r, ok := h.auth.AuthenticateRequest(r)
   if !ok { return nil, huma.Error401Unauthorized("unauthorized") }
   ```
   with
   ```go
   r, _ := humago.Unwrap(input.Ctx)
   if p.orgID(r) == 0 { return nil, huma.Error401Unauthorized("unauthorized") }
   ```
   **This is behaviour-preserving**: the core auth middleware
   (`internal/api/api.go` `api.UseMiddleware`) already authenticates every `/api/`
   op (cookie **or** API token) and injects the result via
   `huma.WithContext(ctx, r2.Context())`, and `auth.Manager.OrgID(r)` reads that
   injected context. The in-handler `AuthenticateRequest` was redundant.
3. `h.cipher.Encrypt/Decrypt` → `p.encrypt(...)` / `p.decrypt(...)` (funcs from
   `ctx.Encrypt`/`ctx.Decrypt`).
4. `h.orgDB(r)` → replicate as a plugin method: `p.db.Where("owner_id = ?", p.orgID(r))`.
5. `h.audit` → `p.audit` (field from `ctx.Audit`).
6. Any feature-private helper the handlers call (`providerFor`, `recordsProvider`,
   `emailBelongsToOrg`, `resolveMailbox`, …) moves into the plugin as a method too.
7. Register the routes in `Mount` with the **exact same** method/path/summary/tags
   copied from `internal/api/api.go` (so the OpenAPI + frontend are unchanged), and
   **delete those same registrations** from `api.go`.

Plugin struct pattern (see `plugins/dns/plugin.go`): fields populated in `Mount`
from the `ctx`, `Describe() plugin.Info { return plugin.Info{Title: "...", Core: true} }`,
`Models()` returns the (still-in-`internal/models`) types, register in `app.New()`
via `a.Use(<pkg>.New())` right after the `dns` line.

## Gotchas already discovered

- **`preflightTableCollisions`** (`app/app.go`) explicitly **allows** a plugin
  declaring a table the core still owns (`models.AllModels()`) — "mirroring a core
  table is the documented convention". So returning `Domain`/`Link`/`Mailbox` from
  a plugin's `Models()` while they remain in `internal/models` is fine (phase 1).
- **Two `plugin.Context` constructions in `app/app.go`** — one in the MCP setup
  path and one in `Run()`. Any Context change must be made in **both**.
- The **api test harness** (`internal/api/api_test.go`) builds `srv = h.Routes()`
  which no longer has the moved routes. A helper `mountCoreDNS(h, db, authMgr,
  cipher)` mounts the dns plugin onto `h.Huma()` so integration tests
  (`comprehensive_api_test`, `coverage_test`, `isolation_test`) keep working with
  no per-test change. **Add analogous `mountCoreMail`/`mountCoreLinks` helpers** and
  call them in both `newTestHandler` and `newTestHandlerWithInstance`.
- Tests that **stub DNS/other resolvers** on the Handler (`h.lookupTXT = …`) must
  move to a **plugin-level harness** (the plugin owns the resolver field now) — see
  the dns step below.
- `internal/models` stays the migration owner in phase 1; **do not touch
  `internal/db` or `models.AllModels()`** (that's phase 2).

## Status: dns plugin — DONE except 3 tests

On the branch already:
- `plugins/dns/{plugin,manager,domains,records,providers}.go` — the full feature,
  compiles, `go build ./...` green.
- Deleted `internal/api/{domains,providers,dns_manager}.go`; removed the DNS route
  block + `providerFor`/`encryptConfig` from `internal/api/{api,helpers}.go`.
- `app/app.go`: registers `dns.New()` in `New()`; `ctx.DNS` now a `lazyDNSManager`
  resolving the `dns.manager` service (both Context constructions updated).
- `plugins/dns/records_test.go` — moved `TestValidateRecord`/`TestNormalizeHost`/
  `TestDNSRecordMappingRoundTrip`. Pass.
- `internal/api/api_test.go` + `invite_dns_test.go` harnesses call `mountCoreDNS`.

**Remaining (immediate first task):** 3 failing tests `TestVerifyDNS`,
`TestVerifyDNSMailHosts`, `TestVerifyDNSLinkHosts` in `internal/api/invite_dns_test.go`.
They stub `h.lookupTXT`/`h.lookupCNAME` (now unused Handler fields) but the route
runs the plugin's real `net` resolvers, so assertions fail. **Fix:** move these 3
tests into `plugins/dns/verify_test.go` with a plugin-level harness that stubs
`p.lookupTXT`/`p.lookupCNAME`:
```go
func newVerifyHarness(t *testing.T) (*Plugin, http.Handler, *gorm.DB) {
    db := openMemDB(t)                     // sqlite :memory:, AutoMigrate models.AllModels()
    mux := http.NewServeMux()
    api := humago.New(mux, huma.DefaultConfig("t", "1.0"))
    p := New()
    reg := plugin.NewRegistry()
    p.Mount(nil, &plugin.Context{
        Huma: api, DB: db,
        OrgID:   func(*http.Request) uint { return 1 }, // fixed org, no session needed
        Audit:   func(*http.Request, string, string, uint, map[string]any) {},
        Encrypt: func(b []byte) (string, error) { return string(b), nil },
        Decrypt: func(s string) ([]byte, error) { return []byte(s), nil },
        Provide: reg.Provide, Lookup: reg.Lookup,
    })
    return p, mux, db
}
```
Seed the `models.Domain` with `OrgID: 1`, set `p.lookupTXT = …`/`p.lookupCNAME = …`
after `Mount`, `GET /api/domains/{id}/verify-dns` via `mux.ServeHTTP` (no cookies —
`OrgID` returns 1). Then delete the 3 tests from `invite_dns_test.go` (and its now
-unused `lookupTXT` stubs). `go test ./... -race` should be green → **dns step done**.
Then optionally remove the dead `lookupTXT`/`lookupCNAME` fields from `api.Handler`.

## mail plugin — TODO (package `plugins/mail`)

Source: `internal/api/mail.go` (~29KB), `internal/api/smtp.go`. Models (stay in
`internal/models`): `Mailbox`, `Email`, `Attachment`, `SMTPSender`.

**Move to `plugins/mail` (dashboard CRUD only):**
- Routes (copy exact defs from `api.go`, delete there):
  `/api/mailboxes` (GET/POST/PUT/DELETE), `/api/emails` (GET list, POST read-all,
  GET {id}, GET {id}/raw, PUT {id}, DELETE {id}, POST send), `/api/smtp-senders`
  (GET/POST/PUT/DELETE).
- Handler methods + their private helpers: `emailBelongsToOrg`, `resolveMailbox`,
  `isReservedMailbox`, `mailHostDisabled`, `wrapLinksInEmail` and the SMTP-sender
  handlers. Convert per the recipe.
- Plugin fields: `db, orgID, audit, encrypt, decrypt` + `sendLimiter` (copy the
  `rateLimiter` used; it's org-keyed) + `getWorkspaceSetting` (from
  `ctx.GetWorkspaceSetting`, for the mail-host settings the send path reads).
- `h.isReservedSlug` (used once in the send/link path) — replicate the small
  settings read locally, or read via `ctx.GetWorkspaceSetting`.

**STAYS in `internal/api` for phase 1 (do NOT move):**
- The **inbound webhook** handlers (`/api/webhook/{orgSlug}/email/inbound|bounce/{token}`)
  and `emitEmail`/`OnEmail` dispatch (`api.go:81`). They own the `OnEmail` seam
  Pro `ai` subscribes to; inverting it is phase 2. They read `models.Email`/`Mailbox`
  directly, unaffected by the CRUD move.
- `a.sendMail` (app.go) reads `models.SMTPSender` — unchanged.

**Tests:** add `mountCoreMail` to the api test harness (mirror `mountCoreDNS`);
move any mail test that stubs a plugin-owned field into `plugins/mail`. Verify the
send path (`POST /api/emails/send`, which calls `wrapLinksInEmail`) still works —
in phase 1 `wrapLinksInEmail` moves *with* mail (it reads `models.Link`), so no
cross-plugin service is needed yet.

## links plugin — TODO (package `plugins/links`)

Source: `internal/api/links.go` (~18KB). Models (stay): `Link`, `LinkEvent`.

**Move to `plugins/links` (dashboard CRUD only):**
- Routes (copy exact, delete from `api.go`): `/api/links` (GET, POST), `/api/links/export.csv`,
  `/api/links/metadata`, `/api/links/{id}` (GET/PUT/DELETE), `/api/links/{id}/stats`,
  `/api/links/{id}/qr`.
- Handler methods + helpers. Plugin fields: `db, orgID, audit` + `cfg` (it reads
  one or two config values — pass what's needed via `New(...)` params or replicate
  from `ctx` settings) + `isReservedSlug` (replicate the settings read) + `queue`
  usage in the CRUD path: check what `h.queue` is used for in `links.go` (1 call) —
  if it's click-analytics enqueue it belongs to the **redirect** path (stays in
  core); if a link CRUD side-effect, expose a minimal enqueue via `ctx` in phase 1
  or keep that one action in core.

**STAYS in `internal/api`/`internal/shortlink` for phase 1 (do NOT move):**
- The **root `/{slug}` redirect engine** (`internal/shortlink`) and **click
  recording** — they read `models.Link`/`Domain` directly and are wired in
  `app.go`. Moving them (with `HandleRoot`) is phase 2.

**Tests:** add `mountCoreLinks`; keep `TestNormalizeTarget` etc. with whatever
package their tested function ends up in (move the function → move the test).

## Finish line

`go build ./...` and `go test ./... -race` green; `gofmt -w`; `npx tsc --noEmit`
in `web/` (frontend untouched, should pass). Do **not** build/commit
`webembed/dist` (CI owns it). Open the single PR for `refactor/core-feature-plugins`.
Phase 2 (move models out of `internal/models`, move the redirect/inbound/OnEmail
runtime + their `Context` additions) is a **separate follow-up PR**.
