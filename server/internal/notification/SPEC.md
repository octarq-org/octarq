# SPEC: Notification Core Engine (`server/internal/notification`)

## 1. Overview & Architecture
`server/internal/notification` is the central multi-channel notification engine of Octarq Core. It provides:
- Channel registry SPI implementing `plugin.NotificationChannel` (`in_app`, `email`, `telegram`, `webhook`).
- Tenant-scoped channel credential decryption with tenant isolation fallback.
- Event pattern routing and per-user notification preferences (`security.*`, `*`, etc.).
- Asynchronous dispatching with exponential backoff and retry limits via `Dispatcher`.
- Strict Pre-v1.0 fail-closed semantics on malformed configurations and unknown channels.

## 2. Exported Contracts & Types
- `Router`: Central registry managing active notification channels, preference resolution, and event broadcasting (`Emit`, `EmitTo`, `SendDirect`, `SendDirectTo`, `SendDirectWithTenant`).
- `Dispatcher`: Asynchronous queue worker delivering notifications with configurable retries and exponential backoff (`SetMaxRetries`, `SetBaseBackoff`, `Dispatch`, `DispatchSync`).
- `Descriptor`: Channel metadata containing type identifier, display name, description, and icon.
- `PreferenceItem`: User channel selection rule mapping an `EventPattern` to preferred channel type names.

## 3. Security & Multi-Tenancy Invariants
- **Tenant Scope Enforcement**: `SendDirectWithTenant(ctx, orgID, typ, cfgJSON, text)` ensures notifications are delivered scoped strictly to the specified tenant ID.
- **Fail-Closed Malformed Config**: `SendDirect` rejects malformed or unparseable JSON channel configurations with an immediate error (`invalid channel config JSON`) rather than silently swallowing the error.
- **No Swallowed Errors**: All delivery failures, configuration parsing issues, and registration anomalies log structured context warnings via `log/slog` or return explicit errors.

## 4. File Structure & Boundaries
- `router.go`: Core router methods and channel lifecycle.
- `router_test.go`: Router and email channel unit tests (<500 LOC).
- `dispatcher.go`: Asynchronous dispatch worker with backoff retry.
- `preferences.go`: User preference persistence and pattern matching.
- `channel_in_app.go`: In-app event spine notification delivery.
- `channel_email.go`: Transactional email delivery channel.
- `channel_telegram.go`: Telegram bot notification delivery.
- `channel_webhook.go`: HTTP webhook callback channel.
- `tenant_isolation_test.go`: Multi-tenant isolation test suite (<500 LOC).
- `tenant_edge_cases_test.go`: Edge cases and direct delivery tests (<500 LOC).
- `notification_characterization_test.go`: Characterization test locking contracts and fail-closed behaviors.
