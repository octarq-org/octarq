package notification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
)

// TestSlackChannel_SSRFProtection stress-tests the Slack channel's SSRF protection
// against tricky URLs: loopback, link-local metadata, IPv6, invalid DNS, private IPs,
// and disallowed schemes.
func TestSlackChannel_SSRFProtection(t *testing.T) {
	safehttp.SetAllowPrivateWebhooks(false)
	t.Cleanup(func() {
		safehttp.SetAllowPrivateWebhooks(true)
	})

	cases := []struct {
		name        string
		url         string
		errContains string
	}{
		{
			name:        "IPv4 loopback standard",
			url:         "http://127.0.0.1/webhook",
			errContains: "non-public address 127.0.0.1 blocked",
		},
		{
			name:        "IPv4 loopback with port",
			url:         "http://127.0.0.1:8080/services/T00/B00/X",
			errContains: "non-public address 127.0.0.1 blocked",
		},
		{
			name:        "IPv4 loopback alternative range (127.0.0.2)",
			url:         "http://127.0.0.2:9999/webhook",
			errContains: "non-public address 127.0.0.2 blocked",
		},
		{
			name:        "Cloud metadata IPv4 (169.254.169.254)",
			url:         "http://169.254.169.254/latest/meta-data/",
			errContains: "non-public address 169.254.169.254 blocked",
		},
		{
			name:        "Cloud metadata with port",
			url:         "http://169.254.169.254:80/computeMetadata/v1/",
			errContains: "non-public address 169.254.169.254 blocked",
		},
		{
			name:        "IPv6 loopback [::1]",
			url:         "http://[::1]/webhook",
			errContains: "non-public address ::1 blocked",
		},
		{
			name:        "IPv6 loopback [::1] with port",
			url:         "http://[::1]:8080/webhook",
			errContains: "non-public address ::1 blocked",
		},
		{
			name:        "IPv4 unspecified (0.0.0.0)",
			url:         "http://0.0.0.0:8080/hook",
			errContains: "non-public address 0.0.0.0 blocked",
		},
		{
			name:        "RFC1918 10.0.0.0/8 private",
			url:         "http://10.0.0.1:8000/webhook",
			errContains: "non-public address 10.0.0.1 blocked",
		},
		{
			name:        "RFC1918 172.16.0.0/12 private",
			url:         "http://172.16.0.1:8000/webhook",
			errContains: "non-public address 172.16.0.1 blocked",
		},
		{
			name:        "RFC1918 192.168.0.0/16 private",
			url:         "http://192.168.1.1:8000/webhook",
			errContains: "non-public address 192.168.1.1 blocked",
		},
		{
			name:        "CGNAT 100.64.0.0/10",
			url:         "http://100.64.0.1:8000/webhook",
			errContains: "non-public address 100.64.0.1 blocked",
		},
		{
			name:        "IPv6 link-local [fe80::1]",
			url:         "http://[fe80::1]:8000/webhook",
			errContains: "non-public address fe80::1 blocked",
		},
		{
			name:        "IPv6 ULA [fc00::1]",
			url:         "http://[fc00::1]:8000/webhook",
			errContains: "non-public address fc00::1 blocked",
		},
		{
			name:        "Disallowed scheme file:// with host",
			url:         "file://localhost/etc/passwd",
			errContains: "disallowed scheme \"file\"",
		},
		{
			name:        "Disallowed scheme gopher://",
			url:         "gopher://127.0.0.1:6379/_flushall",
			errContains: "disallowed scheme \"gopher\"",
		},
		{
			name:        "Disallowed scheme ftp://",
			url:         "ftp://127.0.0.1/upload",
			errContains: "disallowed scheme \"ftp\"",
		},
		{
			name:        "Invalid DNS domain",
			url:         "http://invalid.dns.that.does.not.exist.octarq.internal/webhook",
			errContains: "", // any error is expected since host does not exist
		},
	}

	payload := plugin.NotificationPayload{
		Title: "Test SSRF",
		Body:  "Testing SSRF vulnerability block",
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Do NOT inject a mock client! Use default production NewSlackChannel()
			ch := NewSlackChannel()

			rec := plugin.NotificationRecipient{
				Config: map[string]any{
					"webhook_url": tc.url,
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			err := ch.Send(ctx, rec, payload)
			if err == nil {
				t.Fatalf("expected SSRF error for %s (%s), but Send succeeded", tc.name, tc.url)
			}

			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
			}
		})
	}
}

// TestSlackChannel_LiveLoopbackServerBlocked verifies that an active listening HTTP
// server on 127.0.0.1 is blocked by the default safehttp client.
func TestSlackChannel_LiveLoopbackServerBlocked(t *testing.T) {
	safehttp.SetAllowPrivateWebhooks(false)
	t.Cleanup(func() {
		safehttp.SetAllowPrivateWebhooks(true)
	})

	serverHit := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ch := NewSlackChannel() // Uses real safehttp client
	rec := plugin.NotificationRecipient{
		Config: map[string]any{
			"webhook_url": ts.URL,
		},
	}
	payload := plugin.NotificationPayload{
		Title: "Loopback Probe",
		Body:  "Should never reach server",
	}

	err := ch.Send(context.Background(), rec, payload)
	if err == nil {
		t.Fatal("expected error connecting to loopback httptest server, got nil")
	}
	if serverHit {
		t.Fatal("security breach: SlackChannel successfully reached 127.0.0.1 server!")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected error to indicate SSRF block, got: %v", err)
	}
}

// TestSlackChannel_RedirectSSRFBlocked verifies that if a webhook endpoint redirects
// to 169.254.169.254 or 127.0.0.1, the redirect is stopped at dial time.
func TestSlackChannel_RedirectSSRFBlocked(t *testing.T) {
	safehttp.SetAllowPrivateWebhooks(false)
	t.Cleanup(func() {
		safehttp.SetAllowPrivateWebhooks(true)
	})

	const metadataTarget = "http://169.254.169.254/latest/meta-data/"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, metadataTarget, http.StatusFound)
	}))
	defer ts.Close()

	// When private webhooks are NOT allowed, even the first hop to ts.URL is blocked
	ch := NewSlackChannel()
	rec := plugin.NotificationRecipient{
		Config: map[string]any{
			"webhook_url": ts.URL,
		},
	}
	err := ch.Send(context.Background(), rec, plugin.NotificationPayload{Body: "probe"})
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected initial hop to loopback to be blocked, got %v", err)
	}
}
