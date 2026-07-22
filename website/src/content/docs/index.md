---
title: Octarq
description: The self-hosted operations backend for indie hackers, one-person companies, and small AI-native teams.
template: splash
hero:
  tagline: Octarq is a single Go binary you extend with plugins — the self-hosted operations backend for indie hackers, one-person companies, and small AI-native teams.
  actions:
    - text: Quickstart
      link: /quickstart/
      icon: right-arrow
      variant: primary
    - text: Write a Plugin
      link: /writing-a-plugin/
      icon: document
---

![Octarq Demo](/assets/octarq-demo.gif)

## Why Octarq

**Octarq is a single Go binary you extend with plugins** — the self-hosted operations backend for indie hackers, one-person companies, and small AI-native teams.

Own a domain? Octarq already gives you the things you'd otherwise wire together from three SaaS bills: **short links with analytics, inbound/outbound email, and DNS automation** — each shipped as a first-class plugin, not a locked-in feature. Then you extend it the same way its own core is built: **a small Go interface + a React page = a new tool in your back office.** And because Octarq speaks **MCP**, every plugin you add is instantly drivable by your AI agent (Claude Code, Cursor, Claude Desktop).

> One binary. No CGO. SQLite by default. `go:embed`'d dashboard. Extend without forking.

Think of it as the intersection of three tools you already know:
- **PocketBase's** single-binary, extend-in-an-afternoon developer experience
- applied to **Dub-style** links + real domain/email/DNS infrastructure
- with an **n8n-style** plugin ecosystem for connectors (Telegram, Webhooks, SMS, …)
- and every capability is **agent-native over MCP**.

---

## Batteries Included (Reference Plugins)

These ship in the default build so Octarq is useful on minute one:

- **🔗 Links** — custom/random slugs, geo/device/OS/language routing, expiration & click limits, expired-URL fallbacks, time-series analytics with bot detection, UTM builder, QR codes, tags (optional MaxMind GeoIP).
- **✉️ Mail** — serverless inbound mail via Cloudflare Email Routing (no port 25, no spam daemons), catch-all auto-provisioning, a full client (read/reply/send over your SMTP relays, download raw `.eml`), on-demand AI summaries (BYO key).
- **🌐 DNS** — Cloudflare & DNSPod CRUD, subdomain presets for short-link + email auth (MX/SPF/DKIM), native comment/notes mapping.
- **🏢 Workspaces & RBAC** — isolated multi-tenant orgs, server-enforced roles, invite/onboarding, hashed Bearer API tokens.

Every one of these is a `plugin.Plugin` + `UIPlugin` — the exact same seam your own plugins use.

---

## Agent-Native over MCP

Octarq ships a built-in **MCP server** (`octarq mcp`, over stdio and SSE/stream) so assistants like Claude Code can read and query your instance — `list_links`, `list_mailboxes`, `list_domains`, `export_data`, plus a **guarded read-only SQL tool** (`SELECT`/`WITH` only, row-capped, secrets auto-redacted).

A plugin that implements the optional `MCPProvider` interface exposes its own tools to every connected agent — no extra plumbing. Write a plugin, and your AI agent can drive it.
