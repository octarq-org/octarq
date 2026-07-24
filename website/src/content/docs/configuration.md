---
title: Configuration
description: All configuration is via environment variables.
---

octarq is configured entirely through environment variables — the canonical list
lives in [`.env.example`](https://github.com/octarq-org/octarq/blob/main/.env.example).
The most important ones:

## Core

| Variable | Purpose |
| --- | --- |
| `OCTARQ_SECRET_KEY` | Signs session cookies. **Required.** |
| `OCTARQ_ADMIN_USER` | First admin username (default `admin`). |
| `OCTARQ_ADMIN_PASSWORD` | First admin password. **Required.** |
| `OCTARQ_ADMIN_HOST` | Restrict `/admin` to this hostname (e.g. `admin.example.com`). Unset = served anywhere. |

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
