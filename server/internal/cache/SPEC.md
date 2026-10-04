# Engineering Specification — internal/cache

## 1. Overview
`internal/cache` provides the unified caching provider for Octarq Core and plugins, offering Redis-backed distributed caching with OpenTelemetry metrics/tracing or bounded in-memory caching with TTL eviction.

## 2. Invariants & Multi-Tenant Scoping
- **Scoped Tenant Namespacing**: `ScopedCache` prefixes keys by tenant organization ID (`org:<orgID>:<prefix>:<key>`) whenever `plugin.OrgIDFromContext(ctx)` is present, preventing cross-tenant cache collisions or data bleeding.
- **Fail-Open Read / Fail-Safe Write**: Cache read misses or Redis connection drops safely return `false` to trigger authoritative database fallback queries.
- **Telemetry Instrumentation**: Redis client instances configure OpenTelemetry tracing and metrics instrumentation without silently discarding setup errors.

## 3. Verification
- Validated via `cache_test.go` and `cache_characterization_test.go` under `-race`.
