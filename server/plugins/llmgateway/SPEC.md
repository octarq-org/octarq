# Module Specification: LLM Gateway (`server/plugins/llmgateway`)

## 1. Scope & System Role

The gateway is the single owner of model configuration and model pricing.
Features resolve a provider through it; a billing ledger asks it what a turn
costs. No feature holds a provider key or a price table of its own.

- **Module identifier**: `llmgateway`
- **Plugin kind**: Core (always mounted, never gated, not listed as toggleable)
- **Routes**: `/api/llm-gateway/providers`, `/api/llm-gateway/prices`,
  plus the agent surface `/v1/models`, `/v1/chat/completions`, `/v1/responses`,
  `/v1/messages`, `/v1/messages/count_tokens`, `/v1/opencode/config`, and the
  `/v1beta/models` routes
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
`plugin.ModelPrice{Input, Output, Reasoning, CacheRead, CacheWrite}` or
`plugin.ErrModelUnpriced`. `orgID` scopes the provider row that decides which
provider's price applies.

### 4.1 Catalog

`server/plugins/llmgateway/bundled/models.json.gz` is embedded with `//go:embed`
and parsed into an index keyed by `provider/model`.

- Source: <https://models.dev/api.json>. The repository license is MIT.
- Bundle shape: `{"generated_at", "source", "models": [[modelID, providerID,
  input, output, cacheRead, cacheWrite, reasoning, name], ...]}`. Prices are
  USD per one million tokens. `name` is the display name; an empty name falls
  back to the model ID.
- `server/cmd/pricinggen` writes the bundle (`make pricing`). It keeps only
  models whose `cost.input` and `cost.output` are non-negative numbers, skips
  `experimental.modes`, and sorts by provider then model.
- Parse guards: an empty `models` array, a row shorter than eight fields, a
  missing or negative `input`/`output`, or a negative `cache_read`,
  `cache_write`, or `reasoning` rejects the whole file. A reseller row and a
  first-party row for the same model are both kept; the provider decides.
- `reasoning` is display metadata. It is never billed separately: vendors
  already count thinking tokens in the output total.
- A price change arrives with a bundle refresh, not a manual entry. There is no
  per-org price table.

### 4.2 Lookup rules

1. Provider rows win. The store finds every provider row in the workspace whose
   `model` or `cheapModel` equals the requested name. One distinct mapped
   provider answers the lookup; two distinct providers return
   `plugin.ErrModelAmbiguous`.
2. A row maps to a models.dev provider by backend (`claude` and `anthropic` →
   `anthropic`, `gemini` → `google`, `openai` with no base URL → `openai`) or by
   the base URL host (`api.deepseek.com` → `deepseek`, `openrouter.ai` →
   `openrouter`, and the table in `providerid.go`). An unmapped host returns
   `plugin.ErrModelUnpriced`.
3. With no matching row, a name whose first segment is a known provider is
   looked up as `provider/rest`. Otherwise the bare name resolves only when
   every provider that publishes it agrees on the price. Disagreement returns
   `plugin.ErrModelAmbiguous`. When the name is ambiguous, the lookup retries
   with the workspace's default (or sole) provider mapped by `ProviderIDFor`.
   A price from that retry wins. If the retry yields no price, the original
   `plugin.ErrModelAmbiguous` stands.
4. Lookup normalizes case and treats `.` and `-` as equal, then strips one
   trailing `-YYYYMMDD` or `-YYYY-MM-DD` snapshot suffix.
5. A model absent from the catalog returns `plugin.ErrModelUnpriced`.

A catalog parse failure returns `plugin.ErrPricingUnavailable` for every lookup.
`Plugin.Validate` parses the embedded seed at boot and refuses to start on
failure, so a corrupt embed never serves a turn.

### 4.3 Refresh

`Plugin.Start` loads the newest of the embedded seed and
`<os.UserCacheDir()>/octarq/models.json.gz`, then refreshes
<https://models.dev/api.json> every 24 hours. `OCTARQ_PRICING_REFRESH=off` stops
the loop. A failed fetch, parse, or cache write keeps the current catalog; it
never empties it. An `ErrModelUnpriced` lookup triggers at most one asynchronous
refresh every six hours. The cache write is a temp file plus rename. The binary
never reads another application's cache.

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
- A virtual key authenticates every agent route: `/v1/models`,
  `/v1/chat/completions`, `/v1/responses`, `/v1/messages`,
  `/v1/messages/count_tokens`, `/v1/opencode/config`, and the `/v1beta/models`
  routes. The key arrives as `Authorization: Bearer`, `x-api-key`,
  `x-goog-api-key`, or the `key` query parameter, and only a value with the
  virtual-key prefix is treated as one. Any other value falls through to
  `ctx.Guard`. The key is never copied to the provider request; the provider
  receives only the workspace's own credential.
- A virtual-key turn is authorized before the provider call.
  `SpendLedger.AuthorizeTurn` receives the workspace, the spend account, and the
  resolved model: the body model when present, else the provider default. An
  unpriced model or an unavailable price list answers 400; a paused account or
  an exhausted budget answers 402; an unknown account answers 404. The provider
  is not called on a refusal. When `AllowedModels` is not empty, the resolved
  model must match it, or the call answers 403. `count_tokens`, model lists, and
  the opencode config are free: they accept a virtual key, enforce
  `AllowedModels` where a model is named, and record no spend.
- After a 2xx answer, the turn is billed through `SpendLedger.RecordSpend` with
  a context detached from the request and a 10 s timeout. The upstream usage is
  normalized before billing. An OpenAI `prompt_tokens` count already includes
  the cached tokens, so the billed input is `prompt_tokens - cached_tokens`, and
  `cached_tokens` is billed as cache reads. Anthropic `input_tokens` already
  excludes the cache, so it is billed unchanged. `cache_creation_input_tokens`
  is billed as cache writes. A missing usage object records no spend. The
  upstream response id is the turn id. An id longer than 64 characters is
  replaced with a gateway `gw-` id, so the ledger never drops the row.
- The chat body is forwarded unchanged, so tools, `stream`, and
  `response_format` need no per-vendor handling here. When a virtual key is
  present and the body sets `"stream": true`, the gateway merges
  `"stream_options": {"include_usage": true}` into the body before it forwards
  it. Every other field and every other `stream_options` key is kept. A
  non-stream body is forwarded byte for byte. Streaming answers are flushed as
  they arrive, and the gateway keeps a bounded tail of the answer so a usage
  chunk at the end of a long stream is still billed.
- Errors use the OpenAI `{"error":{...}}` shape, because the client parses that
  shape and nothing else.
- The bounds are 16 MiB per chat body, 15 s for the model-list call, and 128
  characters for a provider `model` or `cheapModel` value.
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
  the client uses the figure only for its context meter. A virtual-key turn is
  billed from the provider usage, not from that estimate.
- `count_tokens` answers with the same estimate. It is not billed. A virtual key
  is accepted, and `AllowedModels` is enforced when the body names a model.

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
  `input_tokens` is an estimate, as in 5.2. A virtual-key turn is billed from
  the reporting usage, in both the streaming and the non-streaming path. The
  wire `input_tokens` stays the OpenAI total, cached tokens included;
  `input_tokens_details.cached_tokens` reports the cached subset. The ledger
  bills the uncached input.

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

`GET /v1/opencode/config` (workspace token or virtual key, same guard as
`/v1/models`):

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
- `usageMetadata` carries the provider counts, including
  `cachedContentTokenCount`. A virtual-key `generateContent` or
  `streamGenerateContent` turn is billed from that usage. `promptTokenCount` is
  the total prompt count, cached tokens included; `cachedContentTokenCount` is
  the cached subset. The ledger bills the uncached input. `countTokens` and the
  model list are free, but they accept a virtual key and enforce
  `AllowedModels` where a model is named.

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
| Virtual-key model has no price, or the price list is unavailable | 400 in the protocol error shape; the provider is not called |
| Virtual-key spend account is paused or over budget | 402 in the protocol error shape |
| Virtual-key spend account does not exist | 404 in the protocol error shape |
| Virtual-key model is not in `AllowedModels` | 403 in the protocol error shape |
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
