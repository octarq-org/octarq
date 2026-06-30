# led — link · email · domain

[![CI](https://github.com/Jungley8/led/actions/workflows/ci.yml/badge.svg)](https://github.com/Jungley8/led/actions/workflows/ci.yml)
[![Release image](https://github.com/Jungley8/led/actions/workflows/release.yml/badge.svg)](https://github.com/Jungley8/led/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A self-hosted **short link, mailbox, and DNS management** service in a single Go
binary with an embedded React dashboard. Inspired by [wr.do](https://github.com/oiov/wr.do)
and [dub](https://github.com/dubinc/dub), rebuilt to ship as **one binary / one Docker image**.

> Single-user today. The schema already carries an owner id on every row, so
> multi-user / multi-org (under a commercial license) drops in without a migration.
> *Bootstrap: the admin env user gets its own org (slug derived from `LED_ADMIN_USER`); OAuth users each get their own personal org. The two are always isolated regardless of login order.*

## Features

- **Overview dashboard** — the home page: total clicks / links / mailboxes /
  domains at a glance, a 30-day clicks chart, top links, device & country
  breakdowns, and recent mail.

- **Short links** — custom or random slugs, host picked from your link-enabled
  domains, password protection, expiry **and expired-URL fallback**, **click
  limits**, **advanced routing rules** (by geo, device, os, language), **tags** + archive, a built-in **UTM builder**, one-click **title
  fetch** from the destination, QR codes, copy-to-clipboard, and **basic click
  analytics** (time series + referer / country / device / browser / **bot detection**).
- **Mailboxes** — addresses built from your mail-enabled domains, receive mail via
  Cloudflare Email Routing, read it (with **attachment list** + raw **.eml**
  download), **reply**, **mark-all-read**, and send through **multiple configured SMTP relays**.
- **DNS** — **one-click sync** of every Cloudflare zone + records, full record CRUD
  with **type/text filtering** and **subdomain presets** (short-link / email), all
  behind a provider abstraction (Cloudflare and DNSPod today; Aliyun / Route53 slot
  in). Record notes map to the provider's native comment / remark field.
- **Open API tokens** — issue bearer tokens for the JSON API; every data endpoint
  accepts either a session cookie or `Authorization: Bearer led_…`. Only a SHA-256
  hash is stored, and the raw token is shown once at creation.
- **Notification channels** — receive alerts via Telegram Bot or Webhook when new mail arrives, managed entirely from the dashboard.
- **One binary** — pure-Go SQLite (no cgo), React dashboard embedded via `go:embed`.
  Postgres supported by flipping two env vars.

> The embedded dashboard also ships UI for **Pro** features (VPS, Finance, Inbox AI,
> Storefront…). In this open-source build their backends aren't present, so those
> pages degrade gracefully to a neutral "unavailable" / upgrade note rather than
> erroring — they light up only on the commercial build.

## Quick start

```bash
cp .env.example .env          # set LED_SECRET_KEY + LED_ADMIN_PASSWORD
make release                  # build web + binary  (or: make all)
./led                         # serves dashboard + API + redirects on :8080
```

Or with Docker:

```bash
docker compose up --build     # reads .env, persists data in a volume
```

The default [`Dockerfile`](Dockerfile) builds the dashboard from source (Node)
and the binary (Go) into a distroless image. If you've already built the web
assets (`make web`) — or build them in a separate CI step — use the
[binary-only Dockerfile](deploy/Dockerfile.binary) for a tiny (~19 MB) `scratch`
image with no Node stage:

```bash
docker build -f deploy/Dockerfile.binary -t led:latest .
```

Open `http://localhost:8080` (redirects to `/admin`), sign in with
`LED_ADMIN_USER` / `LED_ADMIN_PASSWORD`.

## Architecture

```
            ┌──────────────────────────── led (single binary) ────────────────────────────┐
  browser → │  host router                                                                  │
            │   ├─ /api/*        → JSON API (auth, links, domains, mailboxes, emails)       │
            │   ├─ /admin/*      → embedded React dashboard (SPA)                            │
            │   └─ /{slug}       → 302 redirect + async click event (root = link namespace) │
            │                                                                                │
            │  GORM ──┬─ SQLite (pure-Go, default)   DNS ─ provider iface ─ Cloudflare API   │
            │         └─ Postgres (optional)         Mail ─ inbound webhook + SMTP sender     │
            └────────────────────────────────────────────────────────────────────────────────┘
                          ▲
   Cloudflare Email Routing → Worker → POST /api/email/inbound  (deploy/cloudflare-email-worker.js)
```

### Routing

- The dashboard lives under **`/admin`** so the entire root namespace belongs to
  short links — `https://go.example.com/abc` is never shadowed by a dashboard route.
- `/` redirects to `/admin/`.
- `LED_ADMIN_HOST` (e.g. `admin.example.com`), when set, restricts `/admin` to
  that host so pure link hosts don't expose the dashboard; unset = served anywhere.
- Reserved slugs (`admin`, `api`, `assets`, plus any you configure in **Settings**)
  can't be used for short links.

## Geo analytics (optional)

Country / region / city in the click breakdowns come from a MaxMind **GeoLite2-City**
database, which led doesn't bundle (licensing + ~60 MB). Bring your own and point
`LED_GEOIP_DB` at it — unset just leaves geo columns blank. You can grab it from a
**no-key community mirror** (jsDelivr / GitHub, auto-updated) or MaxMind directly
with a free key. For both, plus how to wire it into **Docker / Kubernetes**
(including a bake-into-an-image Dockerfile for k8s), see [`deploy/GEOIP.md`](deploy/GEOIP.md).

## Email receiving (Cloudflare)

led receives mail through Cloudflare Email Routing rather than running its own
SMTP server, so no port 25 / MX / anti-spam ops are required:

1. Enable Email Routing on your domain in Cloudflare.
2. Deploy `deploy/cloudflare-email-worker.js` as a Worker; set `LED_ENDPOINT`
   and `LED_TOKEN` (must match the **Inbound Token** you configure in Settings).
3. Point a catch-all route at the Worker.

Mark a domain as **Accept email** in the dashboard; if **Catch-all** is enabled in Settings, mail to any unknown address on it will automatically create a new mailbox.

## AI · MCP server (`led mcp`)

led ships a built-in [Model Context Protocol](https://modelcontextprotocol.io)
server so an AI client — Claude Code, Claude Desktop, Cursor — can read your
self-hosted company backend directly: *"which links got the most clicks?"*,
*"what mail landed today?"*, *"how many SaaS am I paying for?"*.

Run it over stdio (the universal local MCP transport):

```bash
led mcp
```

Then point your client at it. For Claude Desktop, add to its MCP config:

```json
{ "mcpServers": { "led": { "command": "/path/to/led", "args": ["mcp"] } } }
```

It reads the same `.env` / environment as the server (same database).

**Tools (all read-only):** `list_links`, `list_mailboxes`, `list_emails`,
`list_domains`, `export_data`, and `query_db_readonly` — a general-purpose SQL
tool so the AI can compute any metric without a bespoke endpoint.

**Guardrails on `query_db_readonly`** (the one place data reaches an LLM):
only a single `SELECT`/`WITH` runs (writes, `PRAGMA`, `ATTACH` are rejected,
inside a read-only transaction); results are row-capped; and sensitive columns
(password/token hashes, encrypted provider credentials, raw email bodies) are
redacted. Tools are scoped to one operator via `LED_MCP_ORG_ID`; for a
multi-tenant deployment, run one `led mcp` process per tenant.

### LLM provider

AI features (the MCP server's own tools need no LLM, but the Pro Inbox-AI plugin
does) share one importable abstraction, `github.com/Jungley8/led/llmprovider`.
It is **multi-vendor**: the broad set — OpenAI (and any OpenAI-compatible
endpoint via a base URL), Google Gemini, Mistral, Cohere, and Ollama (local) —
is provided by the open-source [langchaingo](https://github.com/tmc/langchaingo)
framework through one adapter; Claude rides the official Anthropic SDK (so the
Opus 4.7+ family works correctly). Switch vendor by name — no per-vendor code.
Defaults: `claude-opus-4-8` for reasoning, `claude-haiku-4-5` for cheap
classification. In the Pro build it is configured from the dashboard (Inbox AI →
*Configure*, key stored encrypted in the DB); `LED_LLM_*` env vars (see
[`.env.example`](.env.example)) are the fallback.

## Configuration

All configuration is via environment variables — see [`.env.example`](.env.example).

## Development

```bash
# terminal 1: API on :8080
LED_SECRET_KEY=dev LED_ADMIN_PASSWORD=dev go run .
# terminal 2: Vite dev server with hot reload, proxies /api → :8080
make dev
```

## Roadmap

- [x] **P0** scaffold — auth, DB abstraction, embedded SPA, Docker
- [x] **P1** short links + advanced routing (geo/device) + analytics + QR
- [x] **P2** DNS management (Cloudflare, multi-provider interface)
- [x] **P3** email — Cloudflare inbound webhook, inbox UI, multiple SMTP senders
- [x] **P4** open API tokens (bearer auth), system notification channels (Telegram, Webhook)
- [x] **P5** multi-tenant / multi-org — `Org` + `User` + `OrgMember` (owner/admin/member),
  tamper-proof org-scoped session, per-org data isolation (tested), org switcher,
  member management with role enforcement, OAuth users get their own org
  - [ ] invited members can currently sign in only via **OAuth** (email-match);
        a set-password / invite-accept flow for password login is still open

## License

[MIT](LICENSE)
