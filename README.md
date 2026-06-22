# led — link · email · domain

A self-hosted **short link, mailbox, and DNS management** service in a single Go
binary with an embedded React dashboard. Inspired by [wr.do](https://github.com/oiov/wr.do)
and [dub](https://github.com/dubinc/dub), rebuilt to ship as **one binary / one Docker image**.

Unlike wr.do, **every entity — links, mailboxes, and DNS records — supports a free-text note.**

> Single-user today. The schema already carries an owner id on every row, so
> multi-user / multi-org (under a commercial license) drops in without a migration.

## Features

- **Short links** — custom or random slugs, per-host links, password protection,
  expiry, QR codes, and **basic click analytics** (time series + referer / country
  / device / browser breakdown via User-Agent + optional GeoIP).
- **Mailboxes** — receive mail on your domains via Cloudflare Email Routing, read
  it in the dashboard, send replies through an SMTP relay.
- **DNS** — manage records through a provider abstraction (Cloudflare today;
  Aliyun / DNSPod / Route53 slot in). Record notes map to the provider's native
  comment field.
- **One binary** — pure-Go SQLite (no cgo), React dashboard embedded via `go:embed`.
  Postgres supported by flipping two env vars.

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

Open `http://localhost:8080`, sign in with `LED_ADMIN_USER` / `LED_ADMIN_PASSWORD`.

## Architecture

```
            ┌──────────────────────────── led (single binary) ────────────────────────────┐
  browser → │  host router                                                                  │
            │   ├─ /api/*        → JSON API (auth, links, domains, mailboxes, emails)       │
            │   ├─ admin host    → embedded React dashboard (SPA)                            │
            │   └─ link host /x  → 302 redirect + async click event                          │
            │                                                                                │
            │  GORM ──┬─ SQLite (pure-Go, default)   DNS ─ provider iface ─ Cloudflare API   │
            │         └─ Postgres (optional)         Mail ─ inbound webhook + SMTP sender     │
            └────────────────────────────────────────────────────────────────────────────────┘
                          ▲
   Cloudflare Email Routing → Worker → POST /api/email/inbound  (deploy/cloudflare-email-worker.js)
```

### Host routing

- `LED_ADMIN_HOST` (e.g. `admin.example.com`) serves the dashboard.
- Any other host pointed at led serves short links: `https://go.example.com/abc`.
- If `LED_ADMIN_HOST` is empty, every host can serve the dashboard and a bare
  `/slug` is resolved as a short link first, falling back to the SPA.

## Email receiving (Cloudflare)

led receives mail through Cloudflare Email Routing rather than running its own
SMTP server, so no port 25 / MX / anti-spam ops are required:

1. Enable Email Routing on your domain in Cloudflare.
2. Deploy `deploy/cloudflare-email-worker.js` as a Worker; set `LED_ENDPOINT`
   and `LED_TOKEN` (= `LED_INBOUND_TOKEN`).
3. Point a catch-all route at the Worker.

Mark a domain as **Accept email** in the dashboard; with `LED_CATCH_ALL=true`,
mail to any address on it auto-creates a mailbox.

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
- [x] **P1** short links + basic analytics + QR
- [x] **P2** DNS management (Cloudflare, multi-provider interface)
- [x] **P3** email — Cloudflare inbound webhook, inbox UI, SMTP send
- [ ] **P4** open API tokens, more DNS providers, Telegram notifications,
  then multi-tenant (commercial license)

## License

[MIT](LICENSE)
