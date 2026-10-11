# Engineering Specification — internal/usagemetric

## 1. Overview
`internal/usagemetric` defines the canonical string identifiers for metered usage consumption recorded into the `usage_aggregates` table. It establishes single-source-of-truth invariants preventing metric name drift across billing, quota, and core telemetry.

## 2. Canonical Metrics
- `Clicks` (`"clicks"`): Short-link redirects counted towards monthly link quota.
- `MailOut` (`"mailOut"`): Outbound recipient messages dispatched through SMTP or transactional APIs.
- `MailIn` (`"mailIn"`): Inbound recipient messages received and processed.
- `RawBytes` (`"mail.raw_bytes"`): Inbound mail storage volume in bytes (reserved for storage billing).

## 3. Architecture Invariants
- **No Hand-Written Literals**: Call sites invoking `RecordUsage` or `recordUsage` must pass these constants.
- **AST Enforced Purity**: `TestRecordUsageCallSitesUseCanonicalMetrics` walks all Go files in `server/` to verify zero non-canonical string literals at usage recording sites.

## 4. Verification
- Validated via AST unit test and `usagemetric_characterization_test.go`.
