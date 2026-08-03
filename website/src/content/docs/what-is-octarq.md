---
title: Introduction
description: A self-hosted back office for link management, custom email routing, and DNS management in a single Go binary.
sidebar:
  order: 1
  group:
    label: "Start"
---


Octarq is a single Go binary that provides short links, email routing, and DNS management with an embedded administration dashboard. It runs from a single Docker container with no external database or message queue required.

:::note
Octarq is an open-source project licensed under the MIT License. You will see `octarq` binaries and `oct_*` API tokens throughout the documentation—this represents the core system.
:::

## Features

- **Core Capabilities (MIT)**: Short links with analytics, inbound and outbound custom email routing, DNS management, API tokens, Webhooks, and built-in Model Context Protocol (MCP) server support.
- **Plugin Architecture**: Extend Octarq by adding community or custom plugins written in Go and React without forking the codebase.

## Architecture

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

