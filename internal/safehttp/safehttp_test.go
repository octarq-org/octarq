package safehttp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDisallowedIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ip       string
		disallow bool
	}{
		{"", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"169.254.169.254", true},       // Link-local IPv4
		{"fe80::1", true},               // Link-local IPv6
		{"224.0.0.1", true},             // Multicast
		{"ff02::1", true},               // Multicast IPv6
		{"10.0.0.1", true},              // Private RFC1918
		{"172.16.0.1", true},            // Private RFC1918
		{"192.168.1.1", true},           // Private RFC1918
		{"fc00::1", true},               // IPv6 ULA
		{"fd00::1", true},               // IPv6 ULA
		{"100.64.0.1", true},            // CGNAT
		{"100.127.255.255", true},       // CGNAT boundary
		{"100.63.255.255", false},       // Outside CGNAT
		{"100.128.0.0", false},          // Outside CGNAT
		{"8.8.8.8", false},              // Public IPv4
		{"1.1.1.1", false},              // Public IPv4
		{"93.184.216.34", false},        // Public IPv4
		{"2606:4700:4700::1111", false}, // Public IPv6
	}

	for _, tt := range tests {
		var parsed net.IP
		if tt.ip != "" {
			parsed = net.ParseIP(tt.ip)
		}
		got := DisallowedIP(parsed)
		if got != tt.disallow {
			t.Errorf("DisallowedIP(%q) = %v, want %v", tt.ip, got, tt.disallow)
		}
	}
}

func TestControl(t *testing.T) {
	t.Parallel()

	err := Control("tcp", "8.8.8.8:80", nil)
	if err != nil {
		t.Errorf("Control(public) expected nil, got %v", err)
	}

	err = Control("tcp", "127.0.0.1:80", nil)
	if err == nil {
		t.Errorf("Control(loopback) expected error, got nil")
	}

	err = Control("tcp", "invalid-address", nil)
	if err == nil {
		t.Errorf("Control(invalid) expected error, got nil")
	}
}

func TestWebhookControl(t *testing.T) {
	SetAllowPrivateWebhooks(false)
	defer SetAllowPrivateWebhooks(false)

	if err := webhookControl("tcp", "127.0.0.1:80", nil); err == nil {
		t.Errorf("webhookControl with allow=false should block loopback")
	}

	SetAllowPrivateWebhooks(true)
	if err := webhookControl("tcp", "127.0.0.1:80", nil); err != nil {
		t.Errorf("webhookControl with allow=true should pass loopback, got %v", err)
	}
}

func TestSMTPControl(t *testing.T) {
	SetAllowPrivateSMTP(false)
	defer SetAllowPrivateSMTP(false)

	if err := SMTPControl("tcp", "127.0.0.1:25", nil); err == nil {
		t.Errorf("SMTPControl with allow=false should block loopback")
	}

	SetAllowPrivateSMTP(true)
	if err := SMTPControl("tcp", "127.0.0.1:25", nil); err != nil {
		t.Errorf("SMTPControl with allow=true should pass loopback, got %v", err)
	}
}

func TestValidateScheme(t *testing.T) {
	t.Parallel()

	valid := []string{"http", "https"}
	for _, s := range valid {
		if err := ValidateScheme(s); err != nil {
			t.Errorf("ValidateScheme(%q) expected nil, got %v", s, err)
		}
	}

	invalid := []string{"file", "gopher", "ftp", "", "javascript"}
	for _, s := range invalid {
		if err := ValidateScheme(s); err == nil {
			t.Errorf("ValidateScheme(%q) expected error, got nil", s)
		}
	}
}

func TestClientCheckRedirect(t *testing.T) {
	t.Parallel()

	client := NewClient(5 * time.Second)
	if client.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be non-nil")
	}

	req, _ := http.NewRequest("GET", "ftp://example.com", nil)
	via := []*http.Request{{}}
	err := client.CheckRedirect(req, via)
	if err == nil {
		t.Errorf("CheckRedirect with scheme ftp should fail")
	}

	req2, _ := http.NewRequest("GET", "http://example.com", nil)
	via5 := make([]*http.Request, 5)
	err = client.CheckRedirect(req2, via5)
	if err == nil {
		t.Errorf("CheckRedirect with 5 redirects should fail")
	}

	via2 := make([]*http.Request, 2)
	err = client.CheckRedirect(req2, via2)
	if err != nil {
		t.Errorf("CheckRedirect valid should pass, got %v", err)
	}
}

func TestGetDisallowedScheme(t *testing.T) {
	t.Parallel()

	client := NewClient(5 * time.Second)
	_, err := Get(context.Background(), client, "file:///etc/passwd", "")
	if err == nil {
		t.Errorf("Get file:///etc/passwd should be blocked by scheme validation")
	}
}

func TestGetWebhookAllowed(t *testing.T) {
	SetAllowPrivateWebhooks(true)
	defer SetAllowPrivateWebhooks(false)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "Custom-UA" {
			t.Errorf("User-Agent header not set correctly, got %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := NewWebhookClient(5 * time.Second)
	resp, err := Get(context.Background(), client, ts.URL, "Custom-UA")
	if err != nil {
		t.Fatalf("Get unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode expected 200, got %d", resp.StatusCode)
	}
}

func TestFetchPageMetaLoopbackBlocked(t *testing.T) {
	t.Parallel()

	title, desc := FetchPageMeta(context.Background(), "http://127.0.0.1/test")
	if title != "" || desc != "" {
		t.Errorf("FetchPageMeta on loopback target should be blocked by SSRF guard, got title=%q desc=%q", title, desc)
	}
}
