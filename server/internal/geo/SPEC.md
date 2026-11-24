# Engineering Specification — internal/geo

## 1. Overview
`internal/geo` resolves client IP addresses to geographic locations (country, region, city) using a MaxMind GeoLite2 `.mmdb` database and parses client User-Agent strings into structured device, browser, and OS metadata.

## 2. Modes
- **Disabled**: `Open("")` returns a resolver with no database. It logs once, starts no goroutine, performs no network call, and `Locate` returns empty strings.
- **Explicit path**: `Open(path)` loads the file at `path` (from `OCTARQ_GEOIP_DB`). A missing or invalid file is a startup error (fail-closed). The database is read into memory once at startup.

`Open` never fetches a database: an empty path disables geo, a non-empty path must load.

## 3. Session country source
`internal/api` chooses the session location in this order:

1. `127.0.0.1` / `::1` → `Localhost`.
2. `CF-IPCountry` header, **only** when `OCTARQ_TRUST_PROXY` is true and the value is a two-letter ISO code other than `XX`. An empty or invalid value is ignored.
3. The mmdb resolver, when one is loaded.
4. Empty location otherwise.

The header is spoofable, so it is honored only behind a trusted proxy. City may be empty; the formatter still emits the country.

## 4. Invariants & Operations
- **Nil-Safe Resolution**: A nil resolver and a disabled resolver return empty strings without error or panics.
- **Dynamic Reloading**: `Load` replaces the in-memory database without stopping existing queries.
- **Plugin Surface**: `plugin.Host.GeoLookup` and `plugin.Host.ParseUA` keep their existing behavior.

## 5. Verification
- Validated via `geo_test.go`, `mmdb_test.go`, and `geo_characterization_test.go` under `-race`.
- `auth_location_test.go` pins the trust-proxy gate and the localhost behavior.
