# Engineering Specification — internal/cron

## 1. Overview
`internal/cron` provides scheduled task registration, cron expression calculation, and distributed mutual exclusion guards (via Redis `SETNX` or local sync mutexes) for background jobs across Octarq Core.

## 2. Invariants & Architecture
- **Distributed Lock Guard**: In multi-instance deployments backed by Redis, jobs obtain a distributed lock key (`cron:guard:<jobName>`) with a bounded TTL before executing.
- **Fail-Closed on Redis Outage**: If Redis returns a network or command error during `SETNX`, the guard strictly fails closed (aborts execution) to prevent split-brain double execution of mutating tasks.
- **Atomic Lua Release**: Redis lock release evaluates a Lua script verifying token ownership before deletion, ensuring a slow task does not delete a lock acquired by a subsequent iteration.
- **Standard 5-Field Expressions**: Evaluates minute, hour, dom, month, dow with fail-closed rejection of malformed or out-of-range specs.

## 3. Verification
- Validated via `engine_test.go`, `guard_test.go`, `schedule_test.go`, and `cron_characterization_test.go` under `-race`.
