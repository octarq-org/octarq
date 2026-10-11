# Engineering Specification — internal/geo

## 1. Overview
`internal/geo` resolves client IP addresses to geographic locations (country, region, city) using MaxMind GeoLite2 databases and parses client User-Agent strings into structured device, browser, and OS metadata.

## 2. Invariants & Operations
- **Nil-Safe Resolution**: When MaxMind databases are unconfigured or downloading, `Locate` returns empty strings without error or panics.
- **Dynamic Hot-Reloading**: Resolvers support background database refreshing and in-memory byte loading without stopping existing queries.
- **License Key Redaction**: MaxMind download errors and URL logs strictly redact license keys from error messages.

## 3. Verification
- Validated via `geo_test.go`, `download_test.go`, and `geo_characterization_test.go` under `-race`.
