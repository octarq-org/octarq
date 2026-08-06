---
title: Configuration
description: All configuration is via environment variables.
sidebar:
  order: 4
  group:
    label: "Start"
---


Octarq is configured entirely through environment variables — the canonical list
lives in [`.env.example`](https://github.com/octarq-org/octarq/blob/main/.env.example).
The primary configuration options are:

## Core

| Variable | Purpose |
| --- | --- |
| `OCTARQ_SECRET_KEY` | Signs session cookies. **Required.** |
| `OCTARQ_ADMIN_USER` | First admin username (default `admin`). |
| `OCTARQ_ADMIN_PASSWORD` | First admin password. **Required.** |
| `OCTARQ_ALLOW_PRIVATE_WEBHOOKS` | Allow webhook & notification delivery to private/loopback IPs. Default `false`. |
| `OCTARQ_ALLOW_PRIVATE_SMTP` | Allow outbound SMTP mail delivery to private/loopback IPs (e.g. local Postfix or Mailhog). Default `false`. |
| `OCTARQ_TRUST_PROXY` | Honour `X-Forwarded-For` / `X-Real-IP` / `X-Forwarded-Proto`. Enable only behind a reverse proxy you control. Default `false`. |

## Hostnames

There is nothing to configure. Absolute links — password reset, email
verification, workspace invites, OAuth `redirect_uri` — are built from the
hostname the request arrived on, and that hostname is accepted only when it
matches a domain registered under **Domains**. A hostname that matches none
produces relative links instead, so a forged `Host` header can never aim a
password-reset link at somebody else's site.

An instance with no registered domain has nothing to check against and uses the
request host as sent; register the domain you serve on to close that gap.

Two things follow from the same request rather than from configuration:

- **Where the dashboard is served.** Every hostname serves `/admin`, except one
  registered as a short-link or mail host — those exist for a workspace's public
  traffic and do not show a login form.
- **The session cookie's `Secure` attribute.** Set when the request arrived over
  HTTPS. `X-Forwarded-Proto` counts only with `OCTARQ_TRUST_PROXY` on.

**OAuth operators:** `redirect_uri` now varies by hostname, so every hostname
you offer social login on must be registered with the provider as
`https://<host>/auth/callback/<provider>`.

## Database

octarq defaults to pure-Go SQLite (no cgo). Flip to Postgres with two env vars:

| Variable | Purpose |
| --- | --- |
| `OCTARQ_DB_DRIVER` | `sqlite` (default) or `postgres`. |
| `OCTARQ_DB_DSN` | Connection string when using Postgres. |

## Email inbound

Inbound mail is delivered by the Cloudflare Email Worker
([`deploy/cloudflare-email-worker.js`](https://github.com/octarq-org/octarq/blob/main/deploy/cloudflare-email-worker.js)).
It needs one variable — set on the **Worker**, not on octarq:

| Variable (Worker) | Purpose |
| --- | --- |
| `OCTARQ_ENDPOINT` | Your **Inbound Webhook URL**, copied from the octarq dashboard (Settings). It already embeds your org's inbound token — no separate token variable. |

## GeoIP (optional)

Geo data for click analytics. Easiest path: set a free MaxMind license key and
octarq auto-downloads GeoLite2-City into its data dir, reusing the cached file on
later starts.

| Variable | Purpose |
| --- | --- |
| `OCTARQ_MAXMIND_LICENSE_KEY` | Free MaxMind key — enables auto-download of GeoLite2-City. |
| `OCTARQ_GEOIP_DB` | Explicit path to an `.mmdb` file — takes precedence over the cached/downloaded one. Unset both = geo disabled. |

## LLM (AI features)

The MCP server's own tools need no LLM, but AI features (such as AI email summaries and AI assistance) can be configured via environment variables using your own provider key (BYOK):

| Variable | Purpose |
| --- | --- |
| `OCTARQ_LLM_PROVIDER` | `claude`, `openai`, `gemini`, `mistral`, `cohere`, or `ollama`. |
| `OCTARQ_LLM_API_KEY` | Your own provider key — octarq never marks up tokens (BYOK). |

:::tip
Defaults are `claude-opus-4-8` for reasoning and `claude-haiku-4-5` for cheap
classification. Switch vendor by name — no per-vendor code.
:::
