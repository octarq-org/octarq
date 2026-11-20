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

var allowPrivateWebhooks atomic.Bool
var allowPrivateSMTP atomic.Bool

func SetAllowPrivateWebhooks(v bool) { allowPrivateWebhooks.Store(v) }

func SetAllowPrivateSMTP(v bool) { allowPrivateSMTP.Store(v) }

func DisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		ip.IsPrivate() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	return false
}

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

func webhookControl(network, address string, rc syscall.RawConn) error {
	if allowPrivateWebhooks.Load() {
		return nil
	}
	if err := Control(network, address, rc); err != nil {
		return fmt.Errorf("%w (set OCTARQ_ALLOW_PRIVATE_WEBHOOKS=true to allow local/LAN targets)", err)
	}
	return nil
}

func SMTPControl(network, address string, rc syscall.RawConn) error {
	if allowPrivateSMTP.Load() {
		return nil
	}
	if err := Control(network, address, rc); err != nil {
		return fmt.Errorf("%w (set OCTARQ_ALLOW_PRIVATE_SMTP=true to allow local/LAN SMTP)", err)
	}
	return nil
}

func NewClient(timeout time.Duration) *http.Client {
	return newClient(timeout, Control)
}

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

func ValidateScheme(scheme string) error {
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("ssrf guard: disallowed scheme %q", scheme)
	}
	return nil
}

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
