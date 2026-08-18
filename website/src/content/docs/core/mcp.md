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

## Tools (all read-only)

`list_links`, `list_mailboxes`, `list_emails`, `list_domains`, and `export_data`.

## Guardrails

This is the one place data reaches an LLM, so it is fenced:

- Every tool is a **read-only projection** over a specific resource. There is no
  general-purpose SQL tool: an arbitrary query reaches arbitrary tables, so it
  cannot carry the workspace predicate that scopes every other tool. The
  capability is absent from the code rather than gated by a flag.
- Each tool **resolves its workspace from the request** and refuses the call when
  none is present, so an unidentified caller is never served a default tenant.
- Over stdio the workspace is the bootstrap tenant (org 1): that process already
  has local access to the whole database, so this is by design. Over HTTP/SSE the
  workspace comes from the caller's API token.
