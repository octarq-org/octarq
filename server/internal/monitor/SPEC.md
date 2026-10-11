# Engineering Specification — internal/monitor

## 1. Overview
`internal/monitor` provides periodic subsystem health evaluation, resource metric collection (database latency, disk capacity, goroutine runtime stats), and the `/api/monitor/health` inspection endpoint for Octarq Core.

## 2. Invariants & Architecture
- **Hierarchical Health Rollup**: Overall system health aggregates individual provider statuses: any `HealthError` forces overall `HealthError` (HTTP 503); any `HealthWarning` without errors yields `HealthWarning` (HTTP 200); all OK yields `HealthOK` (HTTP 200).
- **Non-blocking Provider Collection**: Health collection executes checks sequentially with timeouts, caching the latest report under a read-write lock for zero-overhead API reads.
- **Provider SPI Binding**: System providers implement `plugin.HealthProvider`.

## 3. Verification
- Validated via `api_test.go`, `collector_test.go`, `db_provider_test.go`, `disk_provider_test.go`, and `monitor_characterization_test.go` under `-race`.
