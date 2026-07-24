---
title: Introduction to Octarq
description: A self-hosted control plane for link management, custom email infrastructure, and DNS orchestration in a single Go binary.
---

Octarq is a self-hosted operations control plane that combines **link management, custom domain email routing, and DNS orchestration** into a single compiled Go binary with an embedded SPA administration dashboard. Rebuilt from the ground up to run as a single Docker image, it has no external dependencies, database configuration, or message queues to manage.

:::note
Octarq is an open-source project licensed under the MIT License. You will see `octarq` binaries and `oct_*` API tokens throughout the documentation—this represents the core system.
:::

## SaaS Cost Consolidation

Solo builders and small teams frequently suffer from tool fragmentation, paying multiple monthly fees for single-purpose SaaS subscriptions: link redirection, domain email hosting, server monitors, and spreadsheet expense logs. Octarq collapses these utilities into a single, unified control plane that you host entirely on your own infrastructure.

## Extensible Core & Features

- **Core Capabilities (MIT)**: Short links with analytics, inbound & outbound custom email routing, DNS orchestration, API tokens, Webhooks, and built-in Model Context Protocol (MCP) server support.
- **Composable Architecture**: Features are modular plugins. You can extend Octarq by adding community or custom plugins written in Go and React without forking the codebase.

## Architecture at a glance

```
browser → host router
   ├─ /api/v1/* → JSON API (auth, links, domains, mailboxes, emails)
   ├─ /admin/*  → embedded React dashboard (SPA)
   └─ /{slug}   → 302 redirect + async click event
GORM ─ SQLite (pure-Go, default) or Postgres
DNS  ─ provider interface (Cloudflare, DNSPod)
Mail ─ Cloudflare Email Routing inbound + SMTP send
```

Continue to the [Quick start](/quickstart/).
