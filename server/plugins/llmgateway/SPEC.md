# Module Specification: LLM Gateway (`server/plugins/llmgateway`)

## 1. Scope & System Role

The gateway is the single owner of model configuration and model pricing.
Features resolve a provider through it; a billing ledger asks it what a turn
costs. No feature holds a provider key or a price table of its own.

- **Module identifier**: `llmgateway`
- **Plugin kind**: Core (always mounted, never gated, not listed as toggleable)
- **Routes**: `/api/llm-gateway/providers`, `/api/llm-gateway/prices`
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
| GET | `/api/llm-gateway/prices` | List prices |
| PUT | `/api/llm-gateway/prices` | Upsert one price |
| DELETE | `/api/llm-gateway/prices?model=` | Delete one price |

Every route requires the workspace admin role. The credentials behind the
routes spend money.

## 6. Failure Modes

| Condition | Behavior |
|---|---|
| No provider | `ErrNotConfigured`; AI features refuse |
| Unknown backend name | `ErrInvalidProvider` at save time |
| Unseal failure | `ErrInvalidProvider`; the turn is refused, never retried with another key |
| No price for a billed model | `plugin.ErrModelUnpriced` |
| Two providers, no default | `ErrNotConfigured` with "choose a default provider" |
| Org 0 | Refused by `TenantDB` |

## 7. Extension Points

- `plugin.ServiceModelPricing` — consumed by billing ledgers.
- `ctx.SetLLMResolverForOrg` — the gateway installs the resolver the core chat
  handler uses. A build without the gateway therefore has no AI, which is the
  documented fail-closed state.
