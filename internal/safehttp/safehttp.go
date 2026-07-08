// Package safehttp provides an HTTP client hardened against SSRF for any
// server-side fetch or POST of a user-supplied URL (link previews, outbound
// webhooks, notification channels, SNS confirmations).
//
// The guard runs at the dialer level via net.Dialer.Control, which fires with
// the final, already-resolved IP right before the socket connects. Checking
// there (rather than parsing the hostname) closes two holes a naive check
// leaves open: DNS rebinding (a hostname that resolves public on the first
// lookup and private on connect) and redirect-based SSRF (every hop dials
// through the same Control, so a 302 to http://169.254.169.254 is blocked too).
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"syscall"
	"time"
)

// allowPrivateWebhooks, when set, lets the webhook/notification client reach
// private/loopback addresses. It exists because a self-hosted operator may run
// their own webhook receiver on the same box or LAN. It is OFF by default (so
// multi-tenant instances stay protected) and only relaxes the webhook client —
// the link-preview client is always strict. Toggled from config (and tests).
var allowPrivateWebhooks atomic.Bool

// SetAllowPrivateWebhooks opts the webhook/notification client into reaching
// private addresses. Intended for trusted self-hosted deployments.
func SetAllowPrivateWebhooks(v bool) { allowPrivateWebhooks.Store(v) }

// DisallowedIP reports whether connecting to ip would reach a loopback,
// private, link-local, CGNAT, or otherwise non-public address.
func DisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		ip.IsPrivate() { // RFC1918 + IPv6 ULA (fc00::/7)
		return true
	}
	// Carrier-grade NAT 100.64.0.0/10 (RFC 6598) — not covered by IsPrivate.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	return false
}

// Control is the net.Dialer.Control hook that rejects connections to non-public
// IPs. address is "ip:port" with the IP already resolved.
func Control(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ssrf guard: bad address %q", address)
	}
	ip := net.ParseIP(host)
	if DisallowedIP(ip) {
		return fmt.Errorf("ssrf guard: connection to non-public address %s blocked", host)
	}
	return nil
}

// webhookControl is the dialer hook for the webhook/notification client. It is
// like Control but honours the allowPrivateWebhooks opt-out.
func webhookControl(network, address string, rc syscall.RawConn) error {
	if allowPrivateWebhooks.Load() {
		return nil
	}
	return Control(network, address, rc)
}

// NewClient builds an http.Client that blocks non-public destinations (incl.
// across redirects), caps redirects, and enforces the given overall timeout.
func NewClient(timeout time.Duration) *http.Client {
	return newClient(timeout, Control)
}

// NewWebhookClient is like NewClient but its guard can be relaxed for private
// targets via SetAllowPrivateWebhooks (trusted self-hosted webhook receivers).
func NewWebhookClient(timeout time.Duration) *http.Client {
	return newClient(timeout, webhookControl)
}

func newClient(timeout time.Duration, control func(string, string, syscall.RawConn) error) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
				Control: control,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: timeout,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("ssrf guard: too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("ssrf guard: disallowed redirect scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}
}

// ValidateScheme rejects any URL scheme other than http/https before a request
// is dialed (so file://, gopher://, etc. never run).
func ValidateScheme(scheme string) error {
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("ssrf guard: disallowed scheme %q", scheme)
	}
	return nil
}

// Get issues a guarded GET for a user-supplied URL through client.
func Get(ctx context.Context, client *http.Client, rawURL, userAgent string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if err := ValidateScheme(req.URL.Scheme); err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	return client.Do(req)
}
