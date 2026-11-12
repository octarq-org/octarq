---
title: MCP server
description: A built-in Model Context Protocol server so an AI client can read your self-hosted backend.
sidebar:
  order: 4
  group:
    label: "Core Features"
---


Octarq ships a built-in [Model Context Protocol](https://modelcontextprotocol.io)
server, allowing AI clients (such as Claude Code, Claude Desktop, or Cursor) to read your
backend directly: *"which links got the most clicks?"*, *"what mail landed today?"*

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

## Tools in a core-only build

| Tool | What it does |
| --- | --- |
| `octarq_infra__describe_tenant_schema` | List the tenant views and their columns |
| `octarq_infra__query_db_readonly` | Run a read-only SQL query over tenant views |
| `octarq_infra__export_data` | Export the workspace's data |
| `octarq_infra__notify` | Send a notification through a configured channel |

Installed plugins add their own namespaced tools, such as
`octarq_network__list_links` from the links plugin.

## Guardrails

This is the one place data reaches an LLM, so it is fenced:

- **SQL cannot name a table.** `query_db_readonly` resolves every table name
  against the registered `tenant_*` views and rejects the query when a name is
  unknown. Each view carries its own workspace predicate, so the query cannot
  outrun the caller's workspace.
- **Sensitive columns are redacted** on the way out, by column name and by each
  view's own sensitive list.
- **Each tool resolves its workspace from the request** and refuses the call
  when none is present, so an unidentified caller is never served a default
  tenant.
- **Duplicate tool names fail closed** instead of overwriting each other.
- Over stdio the workspace is the bootstrap tenant (org 1): that process already
  has local access to the whole database, so this is by design. Over HTTP/SSE the
  workspace comes from the caller's API token.
