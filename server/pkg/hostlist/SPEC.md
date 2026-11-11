# hostlist Specification

## Package Overview
`github.com/octarq-org/octarq/server/pkg/hostlist` provides data types and normalization utilities for hostname collections across plugins and database models.

## Architecture & Invariants
- Zero internal dependencies: standard library only (`database/sql/driver`, `encoding/json`, `fmt`, `strings`).
- JSON serialization invariant: Empty or nil `HostList` serializes as `"[]"`.
- SQL scanner invariant: Scans JSON byte slice or string into `[]Host`. Null, empty byte slice, or empty string sets receiver to `nil`. Unsupported types or invalid JSON return errors.
- Filtering invariants:
  - `Enabled()` returns a slice of strings for hosts where `Enabled == true`.
  - `Blocks(host)` returns `true` if and only if the host is listed in the receiver and every occurrence has `Enabled == false`.
- Host normalization invariant: `NormalizeHost(host)` returns lowercased, trimmed hostname with port and trailing dots removed, properly handling bracketed IPv6 literals (`[::1]:8080` -> `[::1]`).

## API Contracts
```go
type Host struct {
	Host    string `json:"host"`
	Enabled bool   `json:"enabled"`
}

type HostList []Host

func (l HostList) Value() (driver.Value, error)
func (l *HostList) Scan(v any) error
func (l HostList) Enabled() []string
func (l HostList) Blocks(host string) bool
func NormalizeHost(host string) string
```
