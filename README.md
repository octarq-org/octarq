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

## Email receiving (Cloudflare)

led receives mail through Cloudflare Email Routing rather than running its own
SMTP server, so no port 25 / MX / anti-spam ops are required:

1. Enable Email Routing on your domain in Cloudflare.
2. Deploy `deploy/cloudflare-email-worker.js` as a Worker; set `LED_ENDPOINT`
   and `LED_TOKEN` (must match the **Inbound Token** you configure in Settings).
3. Point a catch-all route at the Worker.

Mark a domain as **Accept email** in the dashboard; if **Catch-all** is enabled in Settings, mail to any unknown address on it will automatically create a new mailbox.

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
- [ ] **P5** multi-tenant / multi-org (commercial license)

## License

[MIT](LICENSE)
