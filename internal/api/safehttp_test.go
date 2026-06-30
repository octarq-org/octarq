package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDisallowedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},       // loopback
		{"::1", true},             // loopback v6
		{"169.254.169.254", true}, // cloud metadata (link-local)
		{"10.0.0.5", true},        // RFC1918
		{"192.168.1.1", true},     // RFC1918
		{"172.16.0.1", true},      // RFC1918
		{"100.64.0.1", true},      // CGNAT
		{"0.0.0.0", true},         // unspecified
		{"fc00::1", true},         // IPv6 ULA
		{"8.8.8.8", false},        // public
		{"1.1.1.1", false},        // public
		{"93.184.216.34", false},  // public (example.com)
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if got := disallowedIP(ip); got != c.blocked {
			t.Errorf("disallowedIP(%s) = %v, want %v", c.ip, got, c.blocked)
		}
	}
}

func TestSafeGetBlocksLoopback(t *testing.T) {
	// A real loopback server — the guard must refuse to connect to it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<title>secret</title>"))
	}))
	defer srv.Close()

	if _, err := safeGet(context.Background(), srv.URL); err == nil {
		t.Fatal("safeGet reached a loopback server — SSRF guard failed")
	}
}

func TestSafeGetRejectsNonHTTPScheme(t *testing.T) {
	for _, u := range []string{"file:///etc/passwd", "gopher://127.0.0.1", "ftp://example.com"} {
		if _, err := safeGet(context.Background(), u); err == nil {
			t.Errorf("safeGet allowed disallowed scheme: %s", u)
		}
	}
}

func TestFetchPageMetaBlocksInternal(t *testing.T) {
	// End-to-end: the title-preview path must return nothing for an internal URL
	// rather than fetching it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<title>internal</title>"))
	}))
	defer srv.Close()

	title, desc := fetchPageMeta(context.Background(), srv.URL)
	if title != "" || desc != "" {
		t.Errorf("fetchPageMeta fetched an internal URL: title=%q desc=%q", title, desc)
	}
}
