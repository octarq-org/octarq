# agentstore — Workforce, Tasks, and Agent Billing

`internal/agentstore` owns the persistence and the rules of the AI workforce:
the agent fleet, the task tickets, the approval gate, and the spend ledger that
accrues agent budgets. Nothing in this package fabricates a fleet, a ticket, or
a price: every row was written through the API or by a real agent turn.

## Tables

| Table | Owner | Notes |
|---|---|---|
| `workforce_agents` | this package | Primary key `(id, owner_id)`. `budget_used` is a materialized counter for the current billing period. |
| `workforce_tasks` | this package | Primary key `(id, owner_id)`. Status follows the ticket state machine. |
| `workforce_task_steps` | this package | Append-only execution log, scoped by `owner_id` and `task_id`. |
| `workforce_model_prices` | this package | Primary key `(owner_id, model)`. Price per million tokens in USD. |
| `workforce_spend_events` | this package | Append-only ledger. Rows are never updated or deleted by the store. |
| `workspace_settings` | shared | `workforce.auto_halt` holds the enforcement switch. Absent means on. |

## Ticket state machine

```
todo ──────────► in_progress ──────────► done
 │                   │   ▲
 │                   ▼   │ approve
 └──► awaiting_approval ┘
        │
        └── reject ──► todo
```

`done` is terminal. A gated ticket leaves `awaiting_approval` only through
`DecideApproval`, which is a compare-and-swap on the current status. A replay
fails with `ErrNotAwaitingApproval`; a lost race fails with `ErrTaskChanged`.

## Billing

### Price source

`workforce_model_prices` is the only price source. A model with no row is
**unpriced**, and an unpriced model refuses the turn (`ErrModelPriceMissing`).
A zero price is a valid price: it marks a model the workspace does not pay for
per token.

Prices are quoted per million tokens. The unit is fixed in code
(`priceTokenUnit`) and never travels in an API payload.

### Cost of one turn

```
cost = inputTokens      * inputPerMTok  / 1_000_000
     + outputTokens     * outputPerMTok / 1_000_000
cost = round(cost, 6)
```

`reasoningTokens` is recorded for audit. Every supported provider already
counts reasoning tokens in its output count, so they are not billed twice.

The cost is frozen into `workforce_spend_events.cost_usd` at write time. A later
price change never rewrites history.

### Period

`workforce_agents.budget_period_start` stamps the calendar month `budget_used`
covers. The boundary is the first instant of the month in UTC.

- The first spend of a new month resets the counter to the new cost; it does
  not add to the previous month.
- Readers (`GetAgent`, `ListAgents`, `GetOverview`) project a counter from an
  earlier month to zero. The row is rewritten by the next spend.
- A zero period means "never stamped". The counter is charged to the current
  month — the fail-closed reading.

### Accrual

`RecordSpend` runs one transaction:

1. Load the agent scoped by `(id, owner_id)`.
2. Resolve the price scoped by `(owner_id, model)`.
3. Insert the ledger row.
4. Set `budget_used` and `budget_period_start`.
5. If auto-halt is on and the new total is at or above the limit, set
   `status = 'paused'` in the same write.

A turn is recorded even when it failed partway: the provider bills the tokens it
generated.

### Enforcement

`AuthorizeTurn` runs before the work starts. It refuses, in order:

| Condition | Sentinel | HTTP |
|---|---|---|
| Agent not in this workspace | `ErrAgentNotFound` | 404 |
| Agent is paused | `ErrAgentPaused` | 409 |
| Model has no price | `ErrModelPriceMissing` | 400 |
| Auto-halt on, spend at or above the limit | `ErrBudgetExceeded` | 409 |
| Governance state unreadable | the read error | 500 |

Auto-halt off makes the budget advisory: spend is still recorded and the
dashboard shows it, but no turn is refused for budget. A paused agent is
refused regardless of the switch, because pause is an explicit operator action.

The gate is a check, not a reservation. Two turns that start together can both
pass and overshoot the limit by one turn. Auto-halt stops the next turn.

### Model selection

The billed model is the model in the request, or the provider default when the
request names none. `workforce_agents.model` is a display label and does not
select the provider route or the price row.

## Producers

| Producer | Source | Route |
|---|---|---|
| Agent chat stream | `chat` | `POST /api/ai/chat/stream` with `agentId` |
| External agent runtime | `runtime` | `POST /api/workforce/agents/{id}/spend` |

The chat stream authorizes before the SSE response opens, so a refusal is a
real HTTP status. It requires the workspace admin role, the same authority the
workforce page needs. It records the turn after the run and before the `done`
event; a failed write is surfaced as an SSE `error` event and logged.

The Copilot drawer is the only UI producer. An admin selects an agent in the
**Bill to** control; the drawer sends `agentId` on each turn and shows the
agent spend. A member does not see the control, and the server refuses a member
request with `403 forbidden`. Refusals carry a stable code the drawer
translates into the next action:

| Code | Status | Next action |
|---|---|---|
| `workforce_agent_not_found` | 404 | Select another agent |
| `workforce_agent_paused` | 409 | Resume the agent |
| `workforce_budget_exceeded` | 409 | Raise the limit |
| `workforce_model_price_missing` | 400 | Add the model price |
| `workforce_spend_not_recorded` | SSE `error` | Check the billing setup |

The ingestion endpoint takes token counts, never a cost. A caller cannot set
what it owes.

## Closed vocabularies

- Agent status: `active`, `idle`, `paused` (caller-settable); `executing` is
  written only by a heartbeat.
- Ticket status: `todo`, `in_progress`, `awaiting_approval`, `done`.
- Ticket priority: `urgent`, `high`, `normal`, `low`.
- Step status: `completed`, `running`, `failed`, `awaiting_approval`.
- Spend source: `chat`, `runtime`.

## Errors

`ErrInvalidSpend` covers an unknown source, an empty or over-long model label,
and a negative token count. `ErrInvalidPrice` covers an empty or over-long
model label and a negative or non-finite amount. The API answers 400 for both.

## Non-goals

- Provider-side reconciliation. The ledger records what the platform reports,
  not what the provider invoices.
- Per-task attribution. A spend event names an agent and a session, not a
  ticket.
