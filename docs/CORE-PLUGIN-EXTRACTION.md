# Extracting links / mail / dns into self-contained Core plugins

Status: design settled, implementation in progress on `refactor/core-feature-plugins`.
Goal: lift the three built-in features out of the monolithic `internal/api.Handler`
god-object into three **`Core: true` plugins** that own their own models and routes
and mount by default — the same plugin contract Pro features already use — with **no
behavioural change** and the full test suite green.

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
