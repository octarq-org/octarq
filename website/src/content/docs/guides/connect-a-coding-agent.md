---
title: Connect a coding agent
description: Point Codex, Claude Code, or another agent at your workspace model provider.
sidebar:
  order: 5
  group:
    label: "Guides"
---

A coding agent can spend the workspace's model provider. The provider key stays in the workspace. The agent holds only a workspace token.

The model list comes from the workspace. Copy a name from that list; never invent one.

## What the workspace serves

| Format | Who uses it | Route |
| --- | --- | --- |
| OpenAI Chat Completions | Cline, aider, opencode, most agents | `POST /v1/chat/completions` |
| OpenAI Responses | Codex 0.167 and later | `POST /v1/responses` |
| Anthropic Messages | Claude Code | `POST /v1/messages` |

All three formats share one list at `GET /v1/models`. The provider supplies the list, so a new model appears without a change here.

## What the agent needs

1. **Base URL**: your workspace address, then `/v1`.
2. **API key**: a workspace token. An admin mints one on the tokens page.
3. **Model**: a name from the list that `GET /v1/models` returns.

The workspace needs a model provider first. Add one in **Settings › Model Gateway**, and keep it as the default.

## Codex

Codex 0.167 no longer loads the chat format. It needs the Responses route.

Edit `~/.codex/config.toml`:

```toml
model = "<a model from the list>"
model_provider = "octarq"

[model_providers.octarq]
name = "octarq"
base_url = "https://<your-workspace>/v1"
env_key = "OCTARQ_TOKEN"
wire_api = "responses"
```

Set `OCTARQ_TOKEN` to a workspace token before you start Codex.

## Claude Code

Claude Code uses the Anthropic Messages format. Set three environment variables:

```bash
export ANTHROPIC_BASE_URL="https://<your-workspace>"
export ANTHROPIC_AUTH_TOKEN="<workspace token>"
export ANTHROPIC_MODEL="<a model from the list>"
```

## opencode

opencode discovers the workspace. It reads `/.well-known/opencode`, asks for a token, and loads every model the provider serves. You type no model name.

1. Mint a workspace token in the console.
2. Run `opencode auth login https://<your-workspace>`.
3. Enter the token when asked.

Set `OCTARQ_TOKEN` first to skip the question:

```bash
export OCTARQ_TOKEN="<workspace token>"
opencode auth login https://<your-workspace>
```

After you add a model in the console, run `opencode reload`. The picker shows the new model.

> **Tip**
>
> `curl https://<your-workspace>/.well-known/opencode` shows the document opencode reads. It holds no workspace data.

## Cline, aider, and other agents

Any agent that uses the OpenAI chat format works the same way:

```text
Base URL:  https://<your-workspace>/v1
API key:   <workspace token>
Model:     <a model from the list>
```

## Check the connection

Ask for the model list with the token:

```bash
curl https://<your-workspace>/v1/models \
  -H "Authorization: Bearer $OCTARQ_TOKEN"
```

A list in `data[]` means the token, the workspace, and the provider key all work. An error names the part that does not.

> **Tip**
>
> A local backend such as Ollama needs no API key. Add it as a provider, then point the agent at the workspace as usual.

