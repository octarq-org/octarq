---
title: Connect a coding agent
description: Point Codex, Claude Code, or another agent at your workspace model provider.
sidebar:
  order: 5
  group:
    label: "Guides"
---

A coding agent can spend the workspace's model provider. The provider key stays in the workspace. The agent holds only a workspace token.

The agent reads the model list from the workspace. Do not type a model name into the agent.

## What the workspace serves

| Format | Who uses it | Route |
| --- | --- | --- |
| OpenAI Chat Completions | Cline, aider, most agents | `POST /v1/chat/completions` |
| OpenAI Responses | Codex 0.167 and later | `POST /v1/responses` |
| Anthropic Messages | Claude Code | `POST /v1/messages` |

All three read the same list from `GET /v1/models`. The list comes from the provider, so a new model appears without a change here.

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

