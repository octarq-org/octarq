# Engineering Specification — internal/ipc

## 1. Overview
`internal/ipc` provides the lightweight daemon control plane for Octarq Core over Unix domain sockets using Connect-RPC and plain JSON codecs. It supports CLI administrative commands (`octarq status`, `octarq reload`, `octarq top`).

## 2. Security & Protocol
- **Socket Permissions**: The socket directory is strictly enforced at `0700` and the socket file at `0600`, preventing non-owner local users from issuing commands.
- **Connect-RPC over JSON**: Eliminates protobuf code generation dependencies by registering custom `jsonCodec`.
- **Endpoints**:
  - `Status`: Exposes version, uptime, PID, goroutine count, and allocated memory.
  - `Reload`: Triggers safe live reconfiguration hooks (`OnReload`).
  - `Metrics`: Aggregates subsystem provider health (`OnMetrics`).

## 3. Failure Modes & Invariants
- **Fail-Closed Unreachability**: An unreachable or inactive daemon returns a hard connection error. Clients never fabricate or assume default runtime statistics.
- **Safe Teardown**: Stale sockets are cleanly removed before binding; server shutdowns have 5-second timeouts with structured error logging.

## 4. Verification
- Validated via `ipc_test.go` and `ipc_characterization_test.go` under `-race`.
