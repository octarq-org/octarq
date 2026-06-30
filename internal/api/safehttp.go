package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// SSRF protection for server-side fetches of user-supplied URLs (the link
// title-preview feature). Without it, a user could point led at internal
// addresses — http://127.0.0.1, the cloud metadata endpoint 169.254.169.254,
// RFC1918 ranges — and use the server as a proxy to probe the private network
// or steal instance credentials.
//
// The guard runs at the *dialer* level via net.Dialer.Control, which fires with
// the final, already-resolved IP right before the socket connects. Checking
// there (rather than parsing the hostname) closes two holes a naive check
// leaves open: DNS rebinding (a hostname that resolves to a public IP on the
// first lookup and a private one on the connect) and redirect-based SSRF (the
// http.Client dials every hop through the same Control, so a 302 to
// http://169.254.169.254 is blocked too).

// disallowedIP reports whether connecting to ip would reach a loopback,
// private, link-local, CGNAT, or otherwise non-public address.
func disallowedIP(ip net.IP) bool {
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

// safeControl is the net.Dialer.Control hook that rejects connections to
// non-public IPs. address is "ip:port" with the IP already resolved.
func safeControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ssrf guard: bad address %q", address)
	}
	ip := net.ParseIP(host)
	if disallowedIP(ip) {
		return fmt.Errorf("ssrf guard: connection to non-public address %s blocked", host)
	}
	return nil
}

// safePreviewClient is the shared client for fetching user-supplied URLs. It
// blocks non-public destinations (incl. across redirects), caps redirects, and
// has tight timeouts so a slow or huge target can't tie up the server.
var safePreviewClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
			Control: safeControl,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
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

// safeGet issues a guarded GET for a user-supplied URL. It rejects any scheme
// other than http/https before dialing (so file://, gopher://, etc. never run).
func safeGet(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("ssrf guard: disallowed scheme %q", req.URL.Scheme)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; led-link-preview/1.0)")
	return safePreviewClient.Do(req)
}
