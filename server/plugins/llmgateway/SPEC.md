# Module Specification: LLM Gateway (`server/plugins/llmgateway`)

## 1. Scope & System Role

The gateway is the single owner of model configuration and model pricing.
Features resolve a provider through it; a billing ledger asks it what a turn
costs. No feature holds a provider key or a price table of its own.

- **Module identifier**: `llmgateway`
- **Plugin kind**: Core (always mounted, never gated, not listed as toggleable)
- **Routes**: `/api/llm-gateway/providers`, `/api/llm-gateway/prices`,
  plus the OpenAI-compatible agent surface `/v1/models`, `/v1/chat/completions`
- **Services provided**: `plugin.ServiceModelPricing` (`llmgateway.pricing`)
- **Host seam used**: `ctx.SetLLMResolverForOrg`

## 2. Data Model

| Table | Key | Columns |
|---|---|---|
| `llm_gateway_providers` | `id` | `org_id`, `name`, `backend`, `api_key_encr` (sealed), `base_url`, `model`, `cheap_model`, `is_default`, timestamps |
| `llm_gateway_prices` | unique `(org_id, model)` | `input`, `output`, `reasoning` (USD per one million tokens) |

Tenant column is `org_id`. Every query goes through
`tenantsql.NewTenantDB(db, orgID, tenantsql.WithTenantColumn("org_id"))`, so a
call with org 0 fails closed instead of leaking rows.

The API key is sealed with `ctx.Host.Crypto()` and is never serialized. The API
returns `hasKey` instead.

## 3. Resolution Rules

`Resolve(orgID)` returns the active provider with its key unsealed.

1. A provider with `is_default` is active.
2. If no provider is default and the workspace has exactly one, that one is
   active.
3. Otherwise the call fails with `ErrNotConfigured`.

There is **no environment fallback**. `OCTARQ_LLM_*` no longer configures the
dashboard; a tenant must never spend the operator's credentials. `llmprovider.FromEnv`
remains only for CLI paths that run outside a workspace.

## 4. Price Contract

`plugin.ModelPricing.Price(ctx, orgID, model)` returns
`plugin.ModelPrice{Input, Output, Reasoning}` or `plugin.ErrModelUnpriced`.

A model without a row is refused rather than billed at zero: a cost ceiling over
a zero price can never trip. A consumer without a price registry receives
`plugin.ErrPricingUnavailable` and must refuse the turn.

## 5. Route Contracts

| Method | Path | Effect |
|---|---|---|
| GET | `/api/llm-gateway/providers` | List providers, default first |
| POST | `/api/llm-gateway/providers` | Create; the first provider becomes default |
| PUT | `/api/llm-gateway/providers/{id}` | Update; an empty `apiKey` keeps the stored key |
| DELETE | `/api/llm-gateway/providers/{id}` | Delete |
| POST | `/api/llm-gateway/providers/{id}/default` | Set default (single transaction) |
| POST | `/api/llm-gateway/providers/{id}/test` | One small completion, 15 s timeout |
| POST | `/api/llm-gateway/models` | Ask a provider for its model list; unsaved form values are allowed |
| GET | `/api/llm-gateway/prices` | List prices |
| PUT | `/api/llm-gateway/prices` | Upsert one price |
| DELETE | `/api/llm-gateway/prices?model=` | Delete one price |

Every route requires the workspace admin role. The credentials behind the
routes spend money.

## 5.1 Agent Model API

Coding agents speak the OpenAI wire format, so the gateway serves it on the plain
mux, behind the token guard rather than the huma admin routes.

| Method | Path | Effect |
|---|---|---|
| GET | `/v1/models` | The provider's own model list, normalised |
| POST | `/v1/chat/completions` | The turn, relayed byte for byte |

- The token decides the workspace: `Host.Session().OrgID(r)`. Org 0 is refused;
  a token never names a workspace it does not belong to.
- `?provider=<id>` selects a non-default provider. No selector means the
  workspace's active provider, resolved by the same `Resolve` rules.
- The model list comes from the provider's `/models`, never from a typed-in
  string or a local table. `POST /api/llm-gateway/models` answers the same list
  for a saved provider or for a form that is still being filled in, so the
  console picker can fill itself before the row exists. OpenAI-shaped (`data[]`) and Ollama-shaped
  (`models[]`) answers are both read. An empty list is an error.
- A `claude` row with no base URL reads Anthropic's own paged list
  (`GET /v1/models`, `x-api-key`, `after_id`) and follows every page. The relay
  still refuses that row, because a native Anthropic endpoint does not speak the
  OpenAI format; discovery reads it anyway, so the operator can choose the
  workspace model with a native Anthropic key instead of typing a model name.
  A base URL on Anthropic's own host is read the same way.
- The chat body is forwarded unchanged, so tools, `stream`, and
  `response_format` need no per-vendor handling here. Streaming answers are
  flushed as they arrive.
- Errors use the OpenAI `{"error":{...}}` shape, because the client parses that
  shape and nothing else.
- The bounds are 16 MiB per chat body and 15 s for the model-list call.
- A backend with no OpenAI surface (`claude`, `gemini`, `mistral`, `cohere`) is
  refused by name unless the row sets a base URL that has one.
- The routes live outside `/api/`, so `internal/server` forwards `/v1/` to the
  API mux. Without that branch the handlers exist but nothing routes to them.

## 5.2 Anthropic Messages API (Claude Code)

Claude Code speaks Anthropic's format, not OpenAI's, so the gateway translates it
rather than relaying it.

| Method | Path | Effect |
|---|---|---|
| POST | `/v1/messages` | One Messages turn, streamed or not |
| POST | `/v1/messages/count_tokens` | The client's context meter |

Translation, both directions:

| Anthropic | OpenAI |
|---|---|
| `system` (string or blocks) | a `system` message |
| text part | `content` part of type `text` |
| image part (`base64` or `url`) | `image_url` part (data URI or URL) |
| assistant `tool_use` | `tool_calls[]` with the id kept |
| user `tool_result` | a `tool` message, emitted before the user message |
| `tools[].input_schema` | `tools[].function.parameters` |
| `tool_choice` auto/any/tool/none | `auto` / `required` / named function / `none` |
| `stop_sequences` | `stop` |
| `finish_reason` | `stop_reason` (`stop`→`end_turn`, `length`→`max_tokens`, `tool_calls`→`tool_use`) |
| provider `tool_call.id` | `tool_use.id`, returned unchanged so the next `tool_result` matches |

Rules:

- The model name reaches the provider unchanged. The operator sets
  `ANTHROPIC_MODEL` to a model the provider serves; a wrong name returns the
  provider's error rather than being silently replaced.
- A block with no OpenAI counterpart (`thinking`, a tool result image) is refused
  with 400. Losing part of the client's context silently would be worse.
- Streaming emits the event sequence the SDK parses: `message_start`,
  `content_block_start` / `content_block_delta` / `content_block_stop`,
  `message_delta`, `message_stop`. Text deltas use `text_delta`; tool arguments
  use `input_json_delta`.
- The provider is asked for `stream_options.include_usage`, so
  `message_delta.usage.output_tokens` is the provider's own count.
  `message_start.usage.input_tokens` is an **estimate** (`estimateTokens`, about
  four characters per token): no provider on this path exposes a tokenizer, and
  the client uses the figure only for its context meter. Nothing bills on it.
- `count_tokens` answers with the same estimate.

## 6. Failure Modes

| Condition | Behavior |
|---|---|
| No provider | `ErrNotConfigured`; AI features refuse |
| Unknown backend name | `ErrInvalidProvider` at save time |
| Unseal failure | `ErrInvalidProvider`; the turn is refused, never retried with another key |
| No price for a billed model | `plugin.ErrModelUnpriced` |
| Two providers, no default | `ErrNotConfigured` with "choose a default provider" |
| Org 0 | Refused by `TenantDB` |
| Agent token names no workspace | 403 in the OpenAI error shape |
| Provider lists no models | 502; the console picker and the agent both fail closed |
| Provider base URL cannot be parsed | 400; nothing is sent |
| A Messages block has no OpenAI counterpart | 400; the turn is refused |
| The provider refuses a Messages turn | 502 with the provider's message |

## 7. Extension Points

- `plugin.ServiceModelPricing` — consumed by billing ledgers.
- `ctx.SetLLMResolverForOrg` — the gateway installs the resolver the core chat
  handler uses. A build without the gateway therefore has no AI, which is the
  documented fail-closed state.
