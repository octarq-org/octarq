---
title: MCP server
description: A built-in Model Context Protocol server so an AI client can read your self-hosted backend.
---

octarq ships a built-in [Model Context Protocol](https://modelcontextprotocol.io)
server, so an AI client — Claude Code, Claude Desktop, Cursor — can read your
self-hosted company backend directly: *"which links got the most clicks?"*, *"what
mail landed today?"*, *"how many SaaS am I paying for?"*

## Running it

Run over stdio, the universal local MCP transport:

```bash
octarq mcp
```

For Claude Desktop, add to its MCP config:

```json
{ "mcpServers": { "octarq": { "command": "/path/to/octarq", "args": ["mcp"] } } }
```

It reads the same `.env` / environment (and database) as the server.

## Tools (all read-only)

`list_links`, `list_mailboxes`, `list_emails`, `list_domains`, `export_data`, and
`query_db_readonly` — a general-purpose SQL tool so the AI can compute any metric
without a bespoke endpoint.

## Guardrails on `query_db_readonly`

This is the one place data reaches an LLM, so it is fenced:

- Only a single `SELECT` / `WITH` runs, inside a **read-only transaction** —
  writes, `PRAGMA`, and `ATTACH` are rejected.
- Results are **row-capped**.
- Sensitive columns (password/token hashes, encrypted provider credentials, raw
  email bodies) are **redacted**.
- Tools are scoped to one operator via `OCTARQ_MCP_ORG_ID`; for multi-tenant, run one
  `octarq mcp` process per tenant.
