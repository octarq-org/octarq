package dnsprovider

import "testing"

func TestToCFDefaultsTTL(t *testing.T) {
	r := Record{Type: "A", Name: "x.example.com", Content: "1.2.3.4", Comment: "note"}
	cf := toCF(r)
	if cf.TTL != 1 {
		t.Errorf("expected default TTL 1 (automatic), got %d", cf.TTL)
	}
	if cf.Comment != "note" {
		t.Errorf("comment not mapped: %q", cf.Comment)
	}
}

func TestToCFPreservesTTL(t *testing.T) {
	if cf := toCF(Record{TTL: 300}); cf.TTL != 300 {
		t.Errorf("expected TTL 300, got %d", cf.TTL)
	}
}

func TestCFRoundtripMapping(t *testing.T) {
	prio := 10
	in := cfRecord{
		ID: "id1", Type: "MX", Name: "example.com", Content: "mail.example.com",
		TTL: 600, Proxied: false, Comment: "the note", Priority: &prio,
	}
	r := fromCF(in)
	if r.ID != "id1" || r.Type != "MX" || r.Content != "mail.example.com" {
		t.Errorf("fromCF basic fields wrong: %+v", r)
	}
	if r.Comment != "the note" {
		t.Errorf("comment not mapped from CF: %q", r.Comment)
	}
	if r.Priority == nil || *r.Priority != 10 {
		t.Errorf("priority not mapped from CF: %v", r.Priority)
	}
	// Round-trip back to CF preserves the user-supplied TTL.
	if cf := toCF(r); cf.TTL != 600 {
		t.Errorf("ttl lost on round-trip: %d", cf.TTL)
	}
}
