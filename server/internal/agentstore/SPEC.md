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
| `workforce_spend_events` | this package | Append-only ledger. Rows are never updated or deleted by the store. Unique index `(owner_id, agent_id, external_id)` makes a replayed runtime turn a no-op. |
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

A price outside `[0, 99999999.9999]` is refused (`ErrInvalidPrice`), the range
the column holds. A non-zero price below the column scale is refused too: the
column would store it as zero and the model would bill as free.

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
2. If the input carries a turn ID, look for a ledger row with the same
   `(owner_id, agent_id, external_id)`. A hit returns `ErrSpendReplayed` before
   the price lookup, so a price that changed or was removed after the first
   report cannot rewrite or block the answer.
3. Resolve the price scoped by `(owner_id, model)`. The model label is trimmed
   before the lookup and stored as the price row spells it.
4. Insert the ledger row. With a turn ID, the insert is
   `ON CONFLICT (owner_id, agent_id, external_id) DO NOTHING`: two reports of
   one turn that pass step 2 together both reach the insert, and only one takes
   a row. The loser returns `ErrSpendReplayed`.
5. Accrue `budget_used` in a single statement, never a read-modify-write: a
   counter stamped with an earlier period restarts at this turn's cost, and a
   current counter adds to the stored value. Two turns that finish together
   both land.
6. Re-read the row. If auto-halt is on and the new total is at or above the
   limit, set `status = 'paused'`.

A turn is recorded even when it failed partway: the provider bills the tokens it
generated. The chat path writes the ledger row on a context that outlives the
request, so a client that closes the tab does not lose the charge.

### Ingest idempotency

A runtime turn must carry a turn ID. The store refuses `source = runtime`
without one with `ErrInvalidSpend`; the HTTP handler refuses it with 400. The
chat path has no turn ID: it writes one row per completed turn, and the unique
index ignores the NULL column, so every chat turn is a new charge.

- Uniqueness scope is `(owner_id, agent_id, external_id)`. The same ID on
  another agent or in another workspace is a different turn.
- The first report owns the charge and the price frozen into it.
- `SpendByTurn(orgID, agentID, turnID)` reads that row back. The HTTP handler
  answers a replay with 200 and the original event; a first report answers 201.
- `idx_workforce_spend_turn` is a unique index. A plain index would let two
  concurrent reports each take a row and a charge.

`budget_used` keeps six decimals, the same scale as `cost_usd`. A coarser
column would round every sub-cent turn away from the ledger sum it is
reconciled against.

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
real HTTP status. It records the turn after the run and before the `done`
event; a failed write is surfaced as an SSE `error` event and logged.

### Default agent

`workspace_settings.workforce.default_agent` names the agent an unattributed
chat turn is billed to. It is the single answer to "who pays for this turn", so
every chat surface reads it rather than keeping its own selection.

- `GET /api/workforce/default-agent` serves any member of the workspace. Both
  verbs answer `{agentId, name}`. The name is resolved from the agent row so a
  member can label the setting without the admin-only fleet list. An orphan
  setting answers the bare id with an empty name; the chat refuses that turn
  with `workforce_agent_not_found` rather than hide the cause behind a 500.
- `PUT /api/workforce/default-agent` is admin-only. An empty `agentId` clears
  the setting; an absent field is a 400, and an agent of another workspace is
  a 404.
- `aiChatStream` fills an absent `agentId` from this setting. An explicit
  `agentId` still requires the admin role: a caller who names an agent spends
  workspace money. A turn that runs on the default needs no role, because an
  admin chose that agent for every member.
- No client sends `agentId`. The drawer picker writes the setting and the chat
  surfaces let the server fill it. A client that sent the default explicitly
  would be refused for a member, which is why attribution stays server-side.
- A failure to read the setting refuses the turn with 500. Running it unbilled
  instead would hide a broken store behind free spend.
- `DeleteAgent` clears a default that pointed at the removed agent. A dangling
  setting would refuse every chat turn with `workforce_agent_not_found`.

Refusals carry a stable code the clients
translate into the next action:

| Code | Status | Next action |
|---|---|---|
| `workforce_agent_not_found` | 404 | Select another agent |
| `workforce_agent_paused` | 409 | Resume the agent |
| `workforce_budget_exceeded` | 409 | Raise the limit |
| `workforce_model_price_missing` | 400 | Add the model price |
| `workforce_spend_not_recorded` | SSE `error` | Check the billing setup |

The ingestion endpoint takes token counts, never a cost. A caller cannot set
what it owes. It requires `turnId` (1 to 64 characters) and answers `201` for a
new turn, `200` with the original event for a replay.

`GET /api/workforce/agents/{id}/spend` reads the ledger, newest first, and the
Budgets tab renders it. A turn that is absent from the ledger was not charged.
The list returns at most 500 rows (default 100); the card reads 100 and says so
when it is full.

The provider breakdown on the Budgets tab groups `workforce_spend_events.model`,
not `workforce_agents.model`: the agent's model is a display label that selects
neither the route nor the price, so summing by it would charge a provider that
never ran the turn.

## Closed vocabularies

- Agent status: `active`, `idle`, `paused` (caller-settable); `executing` is
  written only by a heartbeat.
- Ticket status: `todo`, `in_progress`, `awaiting_approval`, `done`.
- Ticket priority: `urgent`, `high`, `normal`, `low`.
- Step status: `completed`, `running`, `failed`, `awaiting_approval`.
- Spend source: `chat`, `runtime`.

## Errors

`ErrInvalidSpend` covers an unknown source, an empty or over-long model label, a
negative token count, and a runtime turn without a turn ID or with one longer
than 64 characters. `ErrInvalidPrice` covers an empty or over-long model label
and a price outside the column range. The API answers 400 for both.
`ErrSpendReplayed` is not an error the client sees: the handler turns it into
the original event with 200. `ErrSpendTurnNotFound` from `SpendByTurn` means
the ledger row is gone between two statements; the handler answers 500.

## Non-goals

- Provider-side reconciliation. The ledger records what the platform reports,
  not what the provider invoices.
- Per-task attribution. A spend event names an agent and a session, not a
  ticket.
