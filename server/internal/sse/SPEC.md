# SPEC — internal/sse

Tier-2 engineering specification. Not embedded in the binary.

## Architecture

Package `internal/sse` provides bounded, multi-tenant Server-Sent Events (SSE) infrastructure for the core server.
It serves persistent, push-based notification channels to authenticated web clients.

```
+-------------------------------------------------------------+
|                          SSE Hub                            |
|                                                             |
|  Tenant 1                     Tenant 2                      |
|  [Conn 1]  [Conn 2] ...       [Conn 1] ...                  |
|     |         |                  |                          |
|  Queue 16  Queue 16           Queue 16                      |
+-----+---------+------------------+--------------------------+
      ^         ^                  ^
      |         |                  |
Broadcast(Org1, ID)          Broadcast(Org2, ID)
```

### Core Components

- **`Hub`**: Manages subscriber sets partitioned by organization ID (`orgID`). Coordinates non-blocking broadcasts, enforces per-tenant connection caps, and coordinates graceful shutdowns.
- **`Conn`**: Represents a single client stream. Encapsulates a bounded message queue (`queue chan string`, buffer depth 16), an atomic drop counter (`dropped atomic.Int64`), and lifecycle cancellation channels (`done chan struct{}`).
- **`Subscribe(w, r)` / `ServeHTTP(w, r)`**: HTTP handler that runs authentication, CSRF checks, and origin verification before establishing the streaming HTTP connection.
- **Heartbeat Ticker**: Periodic timer emitting keepalive ping comments (`: ping\n\n`) to detect broken client sockets and keep reverse proxies from closing idle connections.

## Concurrency Design

1. **Non-blocking broadcast**:
   - `Broadcast(orgID, eventID)` grabs a read lock (`mu.RLock`) to snapshot tenant subscriber connections.
   - For each connection, message enqueue uses a non-blocking `select`:
     ```go
     select {
     case c.queue <- eventID:
     default:
         c.dropped.Add(1)
         h.dropped.Add(1)
     }
     ```
   - A slow or stalled client cannot block the broadcaster loop, nor can it slow down peer connections within the same tenant or across tenants.
2. **Atomic drop metrics**:
   - Each connection tracks its own dropped events via `dropped atomic.Int64`.
   - The hub maintains a cumulative monotonic counter of all dropped events via `h.dropped atomic.Int64`, which survives individual connection lifetimes.
3. **Thread-safe lifecycle & Idempotent unregistration**:
   - `c.close()` uses `sync.Once` to safely close `c.done` exactly once.
   - `Unregister(c)` cleanly removes the connection under `mu.Lock` and deletes empty tenant buckets to prevent memory leakage.
   - `Drain(ctx)` and `Close()` signal all registered connections to terminate cleanly during shutdown.

## Wire Format

The wire format strictly adheres to the W3C Server-Sent Events specification:
- HTTP Response headers:
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
  - `X-Accel-Buffering: no`
- Initial connection frame:
  ```
  : connected\n\n
  ```
- Heartbeat comment frame (emitted every heartbeat tick):
  ```
  : ping\n\n
  ```
- Event ID frame:
  ```
  id: <eventID>\n\n
  ```

### Identifier-Only Principle

The wire carries event IDENTIFIERS only (`id: <eventID>\n\n`). Payload bodies are never placed on the event stream.
A client receiving an identifier must request the actual resource through authenticated, authorized REST or RPC endpoints. This prevents privilege escalation and guarantees that tenant data isolation is preserved even if stream routing were misconfigured.

## Fail-Closed Security Properties

1. **Authentication Gate**:
   - Every connection must provide valid authentication via `AuthFunc`.
   - If `auth` is nil or returns an error, the request is immediately rejected with HTTP 401 Unauthorized (`ErrNoAuth`).
   - Organization ID `0` is strictly rejected (`ErrZeroOrg`). Unscoped streams are forbidden.
2. **CSRF & Origin Verification**:
   - Browser connections carrying cookies are vulnerable to cross-site event interception.
   - When `Origin` is present, it is checked against `origins`.
   - Ambiguous suffixes (such as `example.com` without a leading dot or protocol scheme) are ignored fail-closed to prevent matching unintended domains like `notexample.com`.
   - Custom `CSRFVerify` hook is executed if defined; failure aborts the connection with HTTP 403 Forbidden.
3. **Payload Sanitization**:
   - Event IDs containing C0 control characters or DEL (`0x00`-`0x1F`, `0x7F`) are rejected with `ErrEventIDInvalid`.
   - `IDFrame` and `CommentFrame` flatten newline characters (`\n`, `\r`) into spaces to prevent HTTP response splitting or SSE field injection.
   - Event IDs exceeding `DefaultMaxEventID` (256 bytes) or consisting of only whitespace are rejected with `ErrEventIDTooBig`.

## Limits and Default Configurations

| Parameter | Default Value | Description |
|---|---|---|
| `DefaultHeartbeat` | `15 * time.Second` | Interval between keepalive ping comment frames. |
| `DefaultBuffer` | `16` | Bounded capacity of each subscriber connection channel. |
| `DefaultTenantCap` | `32` | Maximum concurrent active SSE connections per tenant organization. |
| `DefaultMaxEventID` | `256` bytes | Maximum allowed length for an event identifier token. |

## Failure Modes

| Condition | Result | Behavior |
|---|---|---|
| Unauthenticated request | HTTP 401 | Connection refused with `ErrNoAuth`. |
| Zero tenant org ID (`orgID == 0`) | `ErrZeroOrg` | Registration and broadcast fail immediately. |
| Origin not allowlisted | HTTP 403 | Connection refused with `ErrCSRF`. |
| Ambiguous domain suffix in allowlist | Denied | Suffix skipped fail-closed; does not match. |
| Tenant capacity exceeded (> 32) | HTTP 429 | Connection refused with `ErrTenantFull`. |
| Stream writer is not `http.Flusher` | HTTP 500 | Streaming rejected via `Prepare`. |
| Buffer full on broadcast | Non-blocking drop | Frame dropped; `c.dropped` and `h.dropped` incremented. |
| Event ID contains control characters | `ErrEventIDInvalid` | Broadcast refused; nothing sent. |
| Event ID empty or > 256 bytes | `ErrEventIDTooBig` | Broadcast refused; nothing sent. |
| Client disconnect | Clean termination | `Unregister` executes via `defer`; goroutine exits. |
| Hub shutdown (`Close` / `Drain`) | Clean termination | All active connections marked closed; queues drained. |

## Public Surface

- **Types**:
  - `Hub`: Central SSE coordination hub.
  - `Conn`: Active connection handle.
  - `AuthFunc`: Request authenticator returning tenant `orgID`.
  - `CSRFVerify`: Post-authentication CSRF validator.
  - `Option`: Hub functional configuration option.
- **Constructors & Options**:
  - `New(opts ...Option) *Hub`
  - `WithAuth(fn AuthFunc) Option`
  - `WithOrigins(origins ...string) Option`
  - `WithCSRFVerify(fn CSRFVerify) Option`
  - `WithHeartbeat(d time.Duration) Option`
  - `WithBuffer(n int) Option`
  - `WithTenantCap(n int) Option`
  - `WithMaxEventID(n int) Option`
- **Hub Methods**:
  - `Register(orgID uint64) (*Conn, error)`
  - `Unregister(c *Conn)`
  - `Connections(orgID uint64) int`
  - `Broadcast(orgID uint64, eventID string) error`
  - `Dropped() int64`
  - `Subscribe(w http.ResponseWriter, r *http.Request)`
  - `ServeHTTP(w http.ResponseWriter, r *http.Request)`
  - `Close()`
  - `Drain(ctx context.Context)`
- **Conn Methods**:
  - `OrgID() uint64`
  - `Dropped() int64`
  - `Done() <-chan struct{}`
  - `Queue() <-chan string`
  - `Close()`
- **Frame Helpers**:
  - `Prepare(w http.ResponseWriter) (flusher http.Flusher, ok bool)`
  - `CommentFrame(s string) string`
  - `IDFrame(id string) string`
