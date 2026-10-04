# Engineering Specification — internal/safego

## 1. Overview
`internal/safego` provides robust panic recovery primitives for asynchronous background goroutines across Octarq Core and plugins. It prevents background worker panics from crashing the main process while preserving sentinel panics required for HTTP protocol teardown.

## 2. Interface Contracts
- `Recover(name string)`: Intended for deferred execution at the entry point of long-running worker loops. Recovers arbitrary panics, logs full stack traces via `log/slog.Error`, and swallows the crash.
- `Go(name string, fn func())`: Spawns a new goroutine wrapped with `defer Recover(name)`.

## 3. Failure Modes & Invariants
- **Sentinel Re-Panic**: `http.ErrAbortHandler` is explicitly re-panicked. This invariant ensures standard Go `net/http` connection teardown semantics are preserved without logging false-positive 500 server crashes.
- **Structured Error Tracing**: Any recovered panic emits structured logs with `goroutine` name, `panic` value, and `stack` string.

## 4. Verification
- Verified by unit tests and `safego_characterization_test.go` under `-race`.
