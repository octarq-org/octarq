package api

import (
	"testing"

	"github.com/Jungley8/led/internal/dnsprovider"
	"github.com/Jungley8/led/plugin"
)

// TestDNSRecordMappingRoundTrip ensures the plugin↔dnsprovider record conversion
// preserves every field (so an AI-driven DNS change isn't silently lossy).
func TestDNSRecordMappingRoundTrip(t *testing.T) {
	prio := 10
	in := dnsprovider.Record{
		ID: "r1", Type: "MX", Name: "example.com", Content: "mail.example.com",
		TTL: 600, Proxied: true, Comment: "note", Priority: &prio,
	}
	pr := toPluginRecord(in)
	if pr.ID != in.ID || pr.Type != in.Type || pr.Name != in.Name || pr.Content != in.Content ||
		pr.TTL != in.TTL || pr.Proxied != in.Proxied || pr.Comment != in.Comment ||
		pr.Priority == nil || *pr.Priority != prio {
		t.Errorf("toPluginRecord lost data: %+v", pr)
	}
	back := fromPluginRecord(pr)
	if back != in {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", back, in)
	}
	// Empty-ID record (create) stays empty.
	if fromPluginRecord(plugin.DNSRecord{Type: "A"}).ID != "" {
		t.Error("empty ID should round-trip to empty (create)")
	}
}
