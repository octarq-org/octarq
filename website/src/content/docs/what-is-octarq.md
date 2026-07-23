---
title: Introduction to Octarq
description: A self-hosted control plane for link management, custom email infrastructure, and DNS orchestration in a single Go binary.
---

Octarq is a self-hosted operations control plane that combines **link management, custom domain email routing, and DNS orchestration** into a single compiled Go binary with an embedded SPA administration dashboard. Rebuilt from the ground up to run as a single Docker image, it has no external dependencies, database configuration, or message queues to manage.

:::note
Octarq is built on top of the open-source **octarq** engine (MIT). You will see mentions of `octarq` binaries and `oct_*` API tokens throughout the documentation—this represents the open-core system. Octarq adds the premium Pro and Elite observability and AI automation layers.
:::

## SaaS Cost Consolidation

Solo builders and small teams frequently suffer from tool fragmentation, paying multiple monthly fees for single-purpose SaaS subscriptions: link redirection, domain email hosting, server monitors, and spreadsheet expense logs. Octarq collapses these utilities into a single, unified control plane that you host entirely on your own infrastructure.

## Tier Comparison & Features

| Tier | Included Features & Value |
| --- | --- |
| **Core (OSS, MIT)** | Links, Mail, DNS, API tokens, Webhooks, MCP Server |
| **Pro** | + VPS Monitoring, SSH Vault, FinOps, Multi-User / Multi-Org |
| **Elite** | + AI Inbox, Telegram Briefings, Invoice OCR Ledger, MCP Agent |

The core is a real product, not a crippled demo — you can self-host it forever for
free. Paid tiers unlock the ops and AI layers and are delivered as an offline
license key. See [Plans](https://octarq.org/pricing/) for details.

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
