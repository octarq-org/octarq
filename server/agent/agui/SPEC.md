# Engineering Specification: `agent/agui`

## 1. Role

`agent/agui` projects the agent harness onto the AG-UI event protocol. It is the
**only** chat transport of the product: the dashboard drawer, the command
palette and any future client read the same event stream.

The package owns the translation and nothing else. Authentication, workspace
scoping, billing and HTTP framing live in `internal/api`, which feeds it a
`Sink`.

- **Wire format**: AG-UI (`github.com/ag-ui-protocol/ag-ui/sdks/community/go`).
- **Endpoint name**: `POST /api/ai/chat/stream`. AG-UI is the wire format, not
  the endpoint name. There is no `/api/agui` surface.

## 2. What the projection consumes

`Stream` implements two seams of the harness:

| Interface | Source | Use |
|---|---|---|
| `llmprovider.StreamHandler` | model deltas | `OnThinking`, `OnText` |
| `harness.StepObserver` | the run loop, after the guard | `OnToolStart`, `OnToolEnd` |

`OnToolCall` is deliberately a no-op. Provider tool-call chunks are proposals:
the model can ask for a tool the guard denies or the spec removes. A call only
becomes a protocol event after `executeSteps` accepts and runs it, so the UI
never renders a call that did not happen. A denied call still reaches the
client: it is announced with `OnToolStart` and closed with the guard error as
its result.

## 3. Event sequence

One run, in order:

```
RUN_STARTED
  (THINKING_START, THINKING_TEXT_MESSAGE_START,
   THINKING_TEXT_MESSAGE_CONTENT*, THINKING_TEXT_MESSAGE_END, THINKING_END)?
  (TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT*, TEXT_MESSAGE_END)?
  (TOOL_CALL_START, TOOL_CALL_ARGS, TOOL_CALL_END, TOOL_CALL_RESULT)+
FINISHED → RUN_FINISHED | RUN_ERROR
```

Invariants:

- An open thinking block or text message is closed before a tool call starts,
  and before the run ends, whichever ends first.
- Text events of one message share one `messageId`; ids are derived from the
  run id and a sequence number (`msg-<run8>-<seq>`), so two runs of the same
  thread never collide.
- `TOOL_CALL_RESULT.content` is capped at 16 KiB (`maxToolResultBytes`) and cut
  on a UTF-8 boundary, marked with `\n[truncated]`. A failing tool reports its
  error text as the result; the run continues.
- A sink error (client gone) stops all further emission and is reported by
  `Stream.Err()`. The handler must not bill or answer after that.

`Finish` carries the token usage of the turn in the `RUN_FINISHED.usage`
array: provider, model, input, output, reasoning and total tokens. Per-delta
token counts do not exist on this wire; clients read usage from the terminal
event.

`Say` emits one complete assistant message for a provider without streaming
support. The handler calls it only when the run produced no text at all.

## 4. Request contract (`POST /api/ai/chat/stream`)

| Field | Rule |
|---|---|
| `threadId` | Optional, ≤ 128 chars. Generated when absent. Becomes the harness session id and the spend row's `session_id`. |
| `runId` | Optional, ≤ 64 chars. Generated when absent. Becomes the **billing idempotency key**; a retried turn reuses it and is charged once. |
| `messages` | Required. Roles `user` and `assistant` only; content must be a JSON string. The last message must be from the user and must have text. |
| `agentId` | Optional. Attributes the turn to a workforce agent. Requires the admin role. When absent, the workspace default agent bills the turn; when neither exists, the turn is unbilled. |
| `model`, `thinking` | Optional run options. |

Refused with `400` before the stream opens, never silently dropped:

| Field | Message |
|---|---|
| `tools` | Client tools are not supported; the server compiles its own tool list. |
| `state` | Run state is not supported. |
| `context` | Context entries are not supported. |
| `forwardedProps` | Forwarded properties are not supported. |
| `resume` | Run resume is not supported. |

`null`, `[]` and `{}` count as "no value" for those fields and are accepted.
There is no system-prompt field: the prompt is compiled on the server from the
endpoint registry.

## 5. Failure modes

Before the stream opens, the request is refused with the JSON error envelope
and a stable `code`:

- `401` not authenticated; `400` no active workspace.
- `400` invalid run input (messages, oversized ids, malformed JSON).
- `403` `agentId` sent without the admin role.
- `409` `workforce_budget_exceeded`, `workforce_agent_paused`.
- `404` `workforce_agent_not_found`.
- `400` `workforce_model_price_missing`.
- `500` unwired harness, unreadable budget state, or a published spec that
  fails validation. The spec is resolved before the response starts; an
  invalid spec fails the request instead of degrading to the built-in default.

After the stream opens, the only failure channel is `RUN_ERROR`:

| Case | Code | Notes |
|---|---|---|
| The run failed (provider error, `ErrApprovalRequired`, step ceiling) | empty | `message` carries the harness error. A refused tool call surfaces here as "... requires operator approval". |
| Spend could not be recorded | `workforce_spend_not_recorded` | The turn ran; the budget does not know it. Surfaced, not swallowed. |

A spent turn that replayed is not an error: `ErrSpendReplayed` is tolerated and
the run finishes normally.

Each frame may carry an `id:` line before its `data:` line. The `data:` line is
the event; there are no named `event:` lines.

The handler writes `: ping` comment frames every 15 s so a proxy does not close
a run that spends a long time inside one tool. Clients ignore comment frames.

## 6. Client obligations

- Send one `threadId` per conversation and a fresh `runId` per turn. Reuse the
  same `runId` when retrying a turn.
- Send the full message history each turn; the server keeps no session state
  between requests.
- Stop rendering on `RUN_FINISHED` or `RUN_ERROR`. `RUN_ERROR.message` is the
  sentence to show when the client has no translation for the code.
