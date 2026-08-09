package dns

import (
	"strings"

	"github.com/octarq-org/octarq/internal/models"
)

// normalizeHost cleans a user-supplied host into a bare lowercase hostname
// (no scheme, no path, no trailing dot).
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	host = strings.TrimSuffix(host, ".")
	if i := strings.IndexAny(host, "/:"); i >= 0 {
		host = host[:i]
	}
	return host
}

// hostEntry is a host with its enable flag in create/update payloads.
type hostEntry struct {
	Host    string `json:"host"`
	Enabled *bool  `json:"enabled"`
}

// normalizeHosts cleans and de-duplicates a host list, preserving each host's
// enabled flag (defaulting to enabled). A service (links/mail) is considered
// configured when its host list is non-empty — there is no separate toggle.
func normalizeHosts(hosts []hostEntry) models.HostList {
	seen := map[string]bool{}
	var out models.HostList
	for _, h := range hosts {
		name := normalizeHost(h.Host)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		enabled := true
		if h.Enabled != nil {
			enabled = *h.Enabled
		}
		out = append(out, models.Host{Host: name, Enabled: enabled})
	}
	return out
}
