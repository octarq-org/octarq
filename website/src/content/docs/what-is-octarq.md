---
title: Introduction
description: A self-hosted back office you compose — infrastructure in one Go binary, business features as plugins.
sidebar:
  order: 1
  group:
    label: "Start"
---

Octarq is a self-hosted back office in a single Go binary. The binary is the
platform: identity, workspaces, audit, notifications, jobs, webhooks, MCP and the
dashboard shell. The business features you run on it — short links, mailboxes,
DNS — are plugins. You pick them at build time.

:::note
Octarq is open source under the MIT License. Binaries are `octarq`, API tokens
start with `oct_`.
:::

## What core provides

Core holds no business feature. This is the list:

| Layer | What you get |
| --- | --- |
| Identity | Sign-in, API tokens, workspaces, roles, tenant-scoped database access |
| Operations | Audit log, notifications, background job queue, webhooks, backups |
| Agent | MCP server, per-tool risk levels, read-only SQL over tenant views, tier gating |
| Platform | Plugin SDK for Go and React, embedded dashboard, OpenAPI export, tenant SQL views |

## Business features are plugins

Links, mail, DNS and a 2FA vault live in
[octarq-plugins](https://github.com/octarq-org/octarq-plugins). A useful build
carries the plugins you chose, so Octarq publishes no container image — you build
one:

```bash
OCTARQ_PLUGINS='[{"go":"github.com/octarq-org/octarq-plugins/links","npm":"@octarq/plugin-links"}]' make plugin-build
```

The `docker compose up -d --build` path in the quick start builds the default
composition from the source tree.

## How a plugin attaches

A plugin supplies Go models, HTTP handlers and a React page. Each one mounts on a
named seam, and the platform derives everything else:

```
browser ──▶ host router
             ├─ /api/*        → huma → auth middleware → plugin handler
             ├─ /admin/*      → embedded React shell → plugin routes (registerUIPlugin)
             └─ /{slug}       → plugin mounts its own root route (links)
MCP client ─▶ tool registry  → same plugin handlers, workspace resolved per caller
GORM ──▶ SQLite (pure-Go, default) · Postgres · MySQL
```

The public `plugin` package is the whole contract. A plugin never imports
`internal/*`. Read [Writing a plugin](/writing-a-plugin/) for the API, and
[Composition](/architecture/composition/) for how a build picks plugins.

Continue to the [Quick start](/quickstart/).
