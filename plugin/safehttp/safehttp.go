// Package safehttp provides an HTTP client hardened against SSRF.
//
// # When to use it
//
// Use it for every outbound request whose URL — host, port, or path — is
// influenced by anyone other than the operator of the binary: a tenant, an org
// admin, a webhook payload, an identity provider's discovery document, a
// link-preview target, an imported config. If the destination is a constant or
// comes from process configuration, plain net/http is fine.
//
// Concretely, in-tree consumers are link previews, outbound webhooks,
// notification channels, SNS subscription confirmations and SMTP delivery;
// out-of-tree plugins should reach for it whenever they fetch a URL a user
// typed into a form.
//
// # Why dial-time
//
// The guard runs at the dialer level via net.Dialer.Control, which fires with
// the final, already-resolved IP right before the socket connects.
//
// A pre-flight check of the URL is NOT equivalent and must not be treated as a
// substitute. Resolving the hostname yourself and comparing the result against
// a blocklist leaves two holes open:
//
//   - DNS rebinding / TOCTOU: the name resolves public when you check it and
//     private when the transport actually dials. Only the Control hook sees the
//     IP the kernel will connect to.
//   - Redirects: a public URL can 302 to http://169.254.169.254. Every hop
//     dials through the same Control, so each one is re-checked; a check that
//     runs once on the URL the user supplied sees only the first hop.
//
// Parsing the URL up front still has a job — rejecting file://, gopher:// and
// friends before a request exists (see ValidateScheme) — but it is defence in
// depth layered on top of the dialer, never in place of it.
//
// # Policy
//
// The blocked-address set (loopback, unspecified, link-local, multicast,
// RFC1918, IPv6 ULA, CGNAT) and the http/https scheme allowlist are fixed on
// purpose and deliberately not configurable per call. A guard whose policy each
// consumer can widen is a guard that stops guarding somewhere, quietly. The
// only relaxations are the two instance-wide opt-outs below
// (SetAllowPrivateWebhooks, SetAllowPrivateSMTP), which exist for self-hosted
// operators pointing a webhook or SMTP relay at their own LAN, are off by
// default, and are meant to be set once at startup from configuration — not
// toggled around individual requests.
//
// # Caveats
//
// The client caps connect, TLS-handshake and response-header time and enforces
// the overall timeout the caller passes, but it does not limit response size.
// Wrap the body in an io.LimitReader before reading it.
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

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// allowPrivateWebhooks, when set, lets the webhook/notification client reach
// private/loopback addresses. It exists because a self-hosted operator may run
// their own webhook receiver on the same box or LAN. It is OFF by default (so
// multi-tenant instances stay protected) and only relaxes the webhook client —
// the link-preview client is always strict. Toggled from config (and tests).
var allowPrivateWebhooks atomic.Bool
var allowPrivateSMTP atomic.Bool

// SetAllowPrivateWebhooks opts the webhook/notification client into reaching
// private addresses. Intended for trusted self-hosted deployments.
func SetAllowPrivateWebhooks(v bool) { allowPrivateWebhooks.Store(v) }

// SetAllowPrivateSMTP opts outbound SMTP mail delivery into reaching private
// addresses. Intended for trusted self-hosted deployments.
func SetAllowPrivateSMTP(v bool) { allowPrivateSMTP.Store(v) }

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
	if err := Control(network, address, rc); err != nil {
		return fmt.Errorf("%w (set OCTARQ_ALLOW_PRIVATE_WEBHOOKS=true to allow local/LAN targets)", err)
	}
	return nil
}

// SMTPControl is the dialer hook for outbound SMTP mail delivery. It is
// like Control but honours the allowPrivateSMTP opt-out.
func SMTPControl(network, address string, rc syscall.RawConn) error {
	if allowPrivateSMTP.Load() {
		return nil
	}
	if err := Control(network, address, rc); err != nil {
		return fmt.Errorf("%w (set OCTARQ_ALLOW_PRIVATE_SMTP=true to allow local/LAN SMTP)", err)
	}
	return nil
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
	baseTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
			Control: control,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		DisableKeepAlives:     true,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(baseTransport),
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
