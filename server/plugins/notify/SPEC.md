# notify — Engineering Spec (Tier 2)

> Not embedded in the binary. Single technical source of truth for the `notify`
> core plugin.

## Purpose

Expose one MCP/HTTP tool, `notify`, that delivers a message to the calling
workspace's **configured notification channels** (Settings → Alerts). It is the
core replacement for the MCP tools the community `telegram` / `webhook`
connectors used to provide (`send_telegram`, `send_webhook`): an agent reaches a
human over whatever channel the workspace already configured, with no second
configuration surface.

## Surface

| Field | Value |
|---|---|
| Tool / endpoint name | `notify` |
| HTTP | `POST /api/notify` |
| Risk | `write` |
| Auth | required (tenant from the API token / session) |
| MCP | exposed (`ExposeMCP: true`) |

### Input

```json
{ "message": "string (required)", "title": "string (optional)", "channel": "string (optional)" }
```

### Output

```json
{ "delivered": 2, "channels": ["telegram", "webhook"] }
```

## Behaviour

1. Resolve the workspace from `plugin.OrgIDFromContext(ctx)`. Absent → error.
2. Load `notification_channels` where `owner_id = org AND enabled = true`
   (narrowed by `type = channel` when `channel` is given).
3. For each channel, decrypt the stored config with
   `notification.ConfigPlaintext` (same decryptor the host's `ctx.Notify`
   uses — a build without a registered decryptor is an error, not a passthrough)
   and deliver via `notification.SendDirectWithTenant`.
4. `title` is prefixed to the body as `"<title> — <message>"`.

## Failure modes (fail-closed)

- No workspace in the request → error.
- Blank/whitespace message → error.
- No matching enabled channel → error (never a silent success).
- Any channel delivery/decrypt failure → error naming the channel(s); the
  message may already have reached the others, so the error is explicit rather
  than swallowed.

## Dependencies

- `internal/notification` (router + decryptor) — the process-wide `DefaultRouter`
  is installed at app boot (`server/app/app.go`).
- `internal/models.NotificationChannel`.

## Non-goals

- Not a channel *configurator* — configuration lives in Settings → Alerts
  (`/api/notification-channels`). This plugin only *sends*.
- No per-user preference routing (that is `notification.Emit`, used by event
  producers such as `mail.inbound`); `notify` targets every enabled channel.
