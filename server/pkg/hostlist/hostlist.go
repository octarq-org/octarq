package hostlist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

// Host is a single hostname with an enable flag, so a host can be temporarily
// disabled without losing its configuration.
type Host struct {
	Host    string `json:"host"`
	Enabled bool   `json:"enabled"`
}

// HostList is a []Host persisted as a JSON text column.
type HostList []Host

// Value implements driver.Valuer. An empty or nil slice serializes as "[]".
func (l HostList) Value() (driver.Value, error) {
	if len(l) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal([]Host(l))
	return string(b), err
}

// Scan implements sql.Scanner. Supports string, []byte, and nil.
// An empty string or slice resets the receiver to nil.
func (l *HostList) Scan(v any) error {
	if v == nil {
		*l = nil
		return nil
	}
	var b []byte
	switch t := v.(type) {
	case []byte:
		b = t
	case string:
		b = []byte(t)
	default:
		return fmt.Errorf("HostList: unsupported scan type %T", v)
	}
	if len(b) == 0 {
		*l = nil
		return nil
	}
	var hosts []Host
	if err := json.Unmarshal(b, &hosts); err != nil {
		return err
	}
	*l = hosts
	return nil
}

// Enabled returns only the hostnames that are currently enabled.
func (l HostList) Enabled() []string {
	out := make([]string, 0, len(l))
	for _, h := range l {
		if h.Enabled {
			out = append(out, h.Host)
		}
	}
	return out
}

// Blocks reports whether host is listed but every listing is disabled — i.e.
// traffic to it should be dropped. An unlisted host is not blocked.
func (l HostList) Blocks(host string) bool {
	listed := false
	for _, h := range l {
		if h.Host == host {
			listed = true
			if h.Enabled {
				return false
			}
		}
	}
	return listed
}

// NormalizeHost canonicalises a hostname for comparison: lowercased, trimmed,
// port removed, trailing dot removed. So "Acme.COM:8443" and "acme.com." both
// normalise to "acme.com".
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	// Strip the port. IPv6 literals arrive bracketed ("[::1]:8080"), so cut
	// after the closing bracket rather than at the first colon.
	if i := strings.LastIndex(host, "]"); i >= 0 {
		host = host[:i+1]
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}
