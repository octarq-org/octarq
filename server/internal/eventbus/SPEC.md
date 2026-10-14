# SPEC: `internal/eventbus`

## Purpose

Single in-process event bus for Octarq with two complementary subsystems:

1. **Webhook delivery** — persists and retries outbound webhook deliveries for
   tenant-subscribed endpoints.
2. **In-process event spine + reactor engine** — fan-out `Envelope` values to
   in-process `plugin.EventReactor` implementations, with per-entity
   serialization, debounce, and worker pools.

This package is internal; it exposes no help docs and no user-facing HTTP
surface of its own (Reactors and dashboard helpers plug into core via the
plugin SPI).

## Entry Points

- `Init(*gorm.DB)` — registers the shared DB handle.
- `SetSecretDecryptor(func(string) (string, bool))` — registers how stored
  (encrypted) webhook secrets are unwrapped.
- `Publish(orgID uint, event string, data any)` — fan-out entry point.
- `PublishEnvelope(plugin.Envelope)` — spine entry for in-process events.
- `Subscribe(SubscribeOpts) (<-chan Envelope, cancel func())` — spine consumer.
- `ReactorRegistry` + `RegisterReactor` + `StartReactors` — reactor lifecycle.
- `Deliveries`, `Replay`, `TestSend` — dashboard/read paths.

## Webhook Delivery Pipeline

1. `Publish` resolves subscribed `models.Webhook` rows for `orgID`.
2. For each subscribed hook, builds a `delivery` (creates a `WebhookDelivery`
   row in `pending` state **before** the first HTTP attempt).
3. `deliverWithRetry` runs up to `maxAttempts` (5) with exponential backoff
   (base 1s, capped 5m, full jitter) between attempts.
4. Permanent failures (bad scheme, undecryptable secret) dead-letter immediately.
5. Context cancellation mid-loop dead-letters immediately.
6. Retained slots are bounded by `deliverySem` (16 concurrent) via per-hook
   goroutines, preventing tenant-controlled goroutine/socket leaks.
7. Total loop bounded by `deliveryBudget` (15m) to avoid pinning slots.

## Signing

- `signingSecret` rejects builds with no decryptor and undecryptable stored
  secrets instead of passthrough (Fail-Closed).
- v1 signature: `sha256=<hex HMAC-SHA256(body)>`.
- v2 signature binds timestamp + delivery id + body via `v2Material` so
  replay outside the tolerance window is detectable.

## In-Process Event Spine

- `spineSub` entries are guarded by a global `spMu` (slice) plus per-sub
  `subMu` (closed flag + channel). Ordering rule: never take `subMu` while
  holding `spMu` in a way that blocks cancel reversal. Cancel removes from the
  slice first, then closes the channel under `subMu`, so `spineSend` can never
  send on a closed channel.
- `PublishEnvelope` snapshots the subscriber slice without holding the lock
  during channel ops; a never-blocking fan-out with slow-path
  drain-modify-refill under backpressure.
- `DroppedTotal` exposes cumulative drops as a metrics seam.

## Reactor Engine

- `ReactorRegistry.Start` builds `reactorsByKey` routing and spawns one worker
  goroutine per shard (default `max(GOMAXPROCS, 8)`).
- Shards are selected by `hashEntityKey(entityKey)` (or round-robin when empty)
  to serialize per-entity processing.
- Per-reactor `DebounceReactor` MinInterval checked against a per-reactor
  `reactorState.lastFire` map (amortized eviction every 1k ops).
- Panics / `React` errors are caught and logged; execution is best-effort and
  never blocks the spine (transactional retries are handled by asynq queues).
- `executeReactor` wraps each `React` in a 30s timeout.

## Concurrency & Backpressure

- `maxConcurrentDeliveries = 16` (fan-out cap).
- Per-shard channel capacity 256; overflow is counted as a drop.
- `deliverySem` (16) caps webhook attempts.

## Persistence & Best-Effort Fail-Closed Notes

- Delivery-log DB writes are best-effort: `recordPending` / `recordResult`
  failures are logged but never abort the send (losing the audit row is
  worse than dropping the event). This is an explicit design choice.
- `Init` with `nil` DB disables persistence; publish/webhook send degrades to
  a no-op for the HTTP fan-out but the in-process spine still runs.

## Testing Seams

Package vars are intentionally overridable for deterministic tests:
`maxAttempts`, `baseBackoff`, `maxBackoff`, `maxConcurrentDeliveries`,
`deliverySem`, `sleepFn`, `nowFn`, `newDeliveryID`, `deliveryBudget`,
`httpClient`. Characterization tests should lock these defaults.

## Known Acceptable `_ =` Sites

- `hashEntityKey`: `hash.Write` returns `(int, error)` which is always `nil`.
- `spineRandHex`: `crypto/rand.Read` failure is unrecoverable; message
  includes no secret material.
- `attempt`: `io.Copy` drain is best-effort cleanup.
- `backoffFor`: `rand.Int` failure returns an uncapped duration (logged via
  fallback path not needed; deterministic tests override `baseBackoff`).

## Non-Goals

- No user-facing help docs (internal package).
- No API client / OpenAPI contract ownership (HTTP routes are mounted by core).
- No `SPEC.md` embedding in binary.
EOF
echo written