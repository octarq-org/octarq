# Module Specification: LLM Gateway (`server/plugins/llmgateway`)

## 1. Scope & System Role

The gateway is the single owner of model configuration and model pricing.
Features resolve a provider through it; a billing ledger asks it what a turn
costs. No feature holds a provider key or a price table of its own.

- **Module identifier**: `llmgateway`
- **Plugin kind**: Core (always mounted, never gated, not listed as toggleable)
- **Routes**: `/api/llm-gateway/providers`, `/api/llm-gateway/prices`,
  plus the OpenAI-compatible agent surface `/v1/models`,
  `/v1/chat/completions`, `/v1/responses`
- **Services provided**: `plugin.ServiceModelPricing` (`llmgateway.pricing`)
- **Host seam used**: `ctx.SetLLMResolverForOrg`

## 2. Data Model

| Table | Key | Columns |
|---|---|---|
| `llm_gateway_providers` | `id` | `org_id`, `name`, `backend`, `api_key_encr` (sealed), `base_url`, `model`, `cheap_model`, `is_default`, timestamps |

Tenant column is `org_id`. Every provider query goes through
`tenantsql.NewTenantDB(db, orgID, tenantsql.WithTenantColumn("org_id"))`, so a call
with org 0 fails closed instead of leaking rows. Prices have no table: the
catalog ships inside the binary (section 4).

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
Prices are instance-wide, so `orgID` is unused.

### 4.1 Catalog

`server/plugins/llmgateway/prices.json` is embedded with `//go:embed` and parsed
once (`sync.Once`) into a case-insensitive index of `model_id`.

- Source: <https://aicostbudget.github.io/ai-api-pricing-data/api/v1/prices.json>
- Dataset: `AICostBudget AI API Pricing Dataset`, version and `generated_at`
  copied verbatim into the file.
- Licenses: code MIT, data CC BY 4.0.
- The committed file is a derived subset of the upstream JSON: top-level
  provenance, plus `models[]` holding only `model_id`, `display_name`,
  `provider_id`, and `pricing {input, output, cached_input, unit, currency}`.
  Every upstream model is kept. Evidence objects are dropped to keep the
  binary small. Models are ordered by `model_id` so the file is deterministic.
- A price change arrives with a binary update. There is no manual entry and no
  per-org price table.

### 4.2 Lookup rules

1. Exact `model_id` match, case-insensitive.
2. A requested name that contains `/` also matches the segment after the last
   `/`. That suffix resolves only when exactly one model owns it. Two models
   with one suffix refuse the lookup instead of guessing.
3. A model absent from the catalog returns `plugin.ErrModelUnpriced`.
4. A model present with a `null` input or output price returns
   `plugin.ErrModelUnpriced`: it is refused, never billed at zero.
5. `Reasoning` repeats `Output`. The dataset publishes no reasoning rate, so
   reasoning tokens bill at the output rate.

A catalog parse failure returns `plugin.ErrPricingUnavailable` for every lookup.
`Plugin.Validate` parses the catalog at boot and refuses to start on failure, so
a corrupt embed never serves a turn.

A cost ceiling over a zero price can never trip, which is why an unpriced model
is refused. A consumer without a price registry receives
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
| GET | `/api/llm-gateway/prices` | List the billable models of the bundled catalog; read-only |
| GET | `/.well-known/opencode` | Discovery document; public, names no workspace data |
| GET | `/v1/opencode/config` | opencode config with the provider's models; workspace token |

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

## 5.3 OpenAI Responses API (Codex)

Codex CLI speaks the Responses API. Since 0.167 it refuses `wire_api = "chat"`,
so a gateway without this route cannot serve Codex at all. The provider speaks
Chat Completions, so this is a translation, not a relay.

| Method | Path | Effect |
|---|---|---|
| POST | `/v1/responses` | One Responses turn, streamed or not |

Translation, both directions:

| Responses | Chat Completions |
|---|---|
| `instructions` (string or parts) | a `system` message |
| item `message` role `developer`/`system` | a `system` message, position kept |
| item `message` role `user`, `input_text` / `output_text` | a `user` message with `text` parts |
| item `message` role `user`, `input_image` | an `image_url` part |
| item `function_call` (consecutive) | one `assistant` message with `tool_calls[]` |
| item `function_call_output` | a `tool` message keyed by `call_id` |
| item `reasoning` | dropped: provider-private, never produced here |
| `tools[]` type `function` | `tools[].function` |
| `tools[]` type `namespace` | its nested functions, flattened |
| `tools[]` type `web_search` and other hosted tools | dropped: no Chat Completions form |
| `tool_choice` auto/none/required/named | the same value in the OpenAI shape |
| `max_output_tokens` | `max_tokens` |
| `finish_reason` / `tool_calls` | output items: `message` / `function_call` |
| `prompt_tokens` / `completion_tokens` | `input_tokens` / `output_tokens` |

Rules:

- `previous_response_id` is refused with 400. The gateway stores no responses;
  answering without the stored turn would lose the conversation.
- An input item type with no counterpart (`item_reference`) is refused with 400,
  naming the type. A hosted tool has no such hole: it is dropped, so the model
  simply never calls it, and the agent's own loop is unaffected.
- A `message` item always carries `content`, empty included, in
  `response.output_item.added` and in `response.completed`. Codex 0.167 opens
  its active item from this field: without it, every `output_text.delta` is
  logged as "OutputTextDelta without active item" and dropped.
- Streaming emits `response.created`, `response.output_item.added`,
  `response.content_part.added`, `response.output_text.delta` / `.done`,
  `response.content_part.done`, `response.output_item.done`, and closes with
  `response.completed`. Tool calls emit `response.function_call_arguments.delta`
  / `.done`. Each frame carries the event name in `event:`, in `data.type`, and a
  `sequence_number`. Codex treats a stream without `response.completed` as a
  broken turn and retries it.
- The provider is asked for `stream_options.include_usage`, so the closing
  `usage` is the provider's own count when it reports one. Otherwise
  `input_tokens` is an estimate, as in 5.2.

## 5.4 opencode Discovery

opencode resolves a workspace the way it resolves an identity provider: it
fetches `{origin}/.well-known/opencode` before it holds a credential.

`GET /.well-known/opencode` (public):

```json
{
  "auth": {
    "command": ["sh", "-c", "…prints $OCTARQ_TOKEN, or prompts…"],
    "env": "OCTARQ_TOKEN"
  },
  "remote_config": {
    "url": "{origin}/v1/opencode/config",
    "headers": {"Authorization": "Bearer {env:OCTARQ_TOKEN}"}
  }
}
```

- `auth.command` is argv, not a shell line: opencode spawns it and reads its
  stdout as the credential. The command prints `$OCTARQ_TOKEN` when set, else
  prompts on stderr.
- The document carries no provider, model or key. Discoverability must not
  reveal what a workspace spends, and it is fetched before authentication.

`GET /v1/opencode/config` (workspace token, same guard as `/v1/models`):

```json
{"config": {"provider": {"octarq": {
  "npm": "@ai-sdk/openai-compatible",
  "name": "Octarq",
  "env": ["OCTARQ_TOKEN"],
  "options": {"baseURL": "{origin}/v1"},
  "models": {"<provider model id>": {"name": "<same>"}}
}}}}
```

- `models` is `discoveredModels` for the workspace's active provider (or
  `?provider=<id>`). opencode keeps its own list and never calls
  `/v1/models`, so the map is the only way models reach it — and it is
  regenerated on every fetch, so `opencode reload` picks up a new model.
- A workspace with no provider returns 503; a provider that refuses the list
  returns 502. Both are the OpenAI error shape, and both fail closed: an empty
  provider block would show up in opencode as usable.
- `{origin}` is reconstructed from the request; `X-Forwarded-Proto: https` is
  honoured because the document holds no secret and a TLS-terminating proxy
  would otherwise hand back `http`.

## 5.5 Gemini generateContent API (Gemini CLI)

Gemini CLI speaks Google's protocol and nothing else; it has no OpenAI mode. The
gateway translates that protocol the same way it translates the Messages API:
the provider is called over OpenAI, and the answer is rebuilt.

| Method | Path | Effect |
|---|---|---|
| GET | `/v1beta/models` | The provider's models in the Gemini list shape |
| GET | `/v1beta/models/{model}` | One model, 404 when the provider does not list it |
| POST | `/v1beta/models/{model}:generateContent` | One turn |
| POST | `/v1beta/models/{model}:streamGenerateContent?alt=sse` | The same turn as `data:` frames |
| POST | `/v1beta/models/{model}:countTokens` | Estimate; the provider is not called |

Translation, both directions:

| Gemini | OpenAI |
|---|---|
| `systemInstruction` | a `system` message |
| text part | `text` content part |
| `inlineData` part | `image_url` part as a data URI |
| `functionCall` part | assistant `tool_calls[]` |
| `functionResponse` part | a `tool` message, emitted before the user message |
| `tools[].functionDeclarations` | `tools[].function.parameters` |
| `toolConfig` AUTO/ANY/NONE | `auto` / `required` or the one named function / `none` |
| `maxOutputTokens`, `temperature`, `topP`, `stopSequences` | `max_tokens`, `temperature`, `top_p`, `stop` |
| `responseJsonSchema`, `responseMimeType` | `response_format` `json_schema` or `json_object` |
| `finishReason` | `finish_reason` (`STOP`, `MAX_TOKENS` for `length`, `SAFETY` for `content_filter`) |
| `usageMetadata` | `usage` prompt and completion counts |

- The Gemini protocol gives a function call no id and names the function in the
  response. The gateway mints `call_N` while it walks the history and pairs each
  `functionResponse` with the oldest unanswered call of the same name, so a turn
  with two calls to one function answers in order.
- A `thought` part is not sent upstream: no OpenAI field carries reasoning, and
  replaying it as prose changes the turn. A `fileData` part is refused by name,
  because its URL is only readable by Google's own platform.
- The model in the path is not the model that runs. gemini-cli sends names such
  as `gemini-2.5-pro` and its router sends internal names, while a workspace
  provider serves the model it was configured with, so the turn goes to that
  model; `?model=` names another. The answer reports the model that ran.
- Function arguments are held until the stream closes: they are JSON that is
  only whole at the end, and a Gemini client reads the call as one object. Text
  streams as it arrives.
- `parts` is written as an empty list, never `null`, on every frame: gemini-cli
  reads `parts.length` and throws on null.

## 6. Failure Modes

| Condition | Behavior |
|---|---|
| No provider | `ErrNotConfigured`; AI features refuse |
| Unknown backend name | `ErrInvalidProvider` at save time |
| Unseal failure | `ErrInvalidProvider`; the turn is refused, never retried with another key |
| No price for a billed model | `plugin.ErrModelUnpriced`; a model with a `null` price is refused too |
| Price catalog does not parse | `plugin.ErrPricingUnavailable`; `Validate` stops the boot |
| Two providers, no default | `ErrNotConfigured` with "choose a default provider" |
| Org 0 | Refused by `TenantDB` |
| Agent token names no workspace | 403 in the OpenAI error shape |
| Provider lists no models | 502; the console picker and the agent both fail closed |
| Provider base URL cannot be parsed | 400; nothing is sent |
| A Messages block has no OpenAI counterpart | 400; the turn is refused |
| The provider refuses a Messages turn | 502 with the provider's message |
| A Gemini workspace names no model | 503 `FAILED_PRECONDITION` in the Gemini error shape |
| A Gemini part has no OpenAI counterpart | 400 `INVALID_ARGUMENT` |
| An unknown Gemini action | 404 `NOT_FOUND`; the action list is closed |

## 7. Extension Points

- `plugin.ServiceModelPricing` — consumed by billing ledgers.
- `ctx.SetLLMResolverForOrg` — the gateway installs the resolver the core chat
  handler uses. A build without the gateway therefore has no AI, which is the
  documented fail-closed state.
