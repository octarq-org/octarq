# Engineering Specification — internal/cleanup

## 1. Overview
`internal/cleanup` executes periodic background maintenance for Octarq Core, pruning expired session tokens and purging historical audit logs older than the configurable `data_retention_days` window.

## 2. Invariants & Operations
- **Non-blocking Batch Deletion**: `PruneAuditLogs` queries IDs in chunks of up to 2,000 rows with a 50ms pause between batches, preventing long-held table locks on large SQLite/PostgreSQL tables.
- **Fail-Safe Retention Windows**: Retention days <= 0 or nil database connection immediately no-ops, returning `(0, nil)` without mutating records.
- **Periodic Tickers**:
  - `Start`: Runs plugin cleanup hooks every 24 hours.
  - `StartSessionCleanup`: Runs expired session purge and audit log pruning every 1 hour.
- **Error Observability**: Background errors during log pruning emit structured log events and are not silently discarded.

## 3. Verification
- Validated via `cleanup_test.go` and `cleanup_characterization_test.go` under `-race`.
