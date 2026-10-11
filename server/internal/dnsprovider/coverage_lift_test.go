package dnsprovider

import (
	"context"
	"net/http"
	"testing"
)

func TestCoverageLift_CloudflareUpdateRecord_WithPriority(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/z1/dns_records/r1" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		w.Write([]byte(`{
			"success": true,
			"result": {
				"id": "r1",
				"type": "MX",
				"name": "mail.example.com",
				"content": "1.2.3.4",
				"ttl": 300,
				"priority": 10,
				"comment": "mx record"
			}
		}`))
	})

	priority := 10
	rec, err := c.UpdateRecord(context.Background(), "z1", Record{
		ID:       "r1",
		Type:     "MX",
		Name:     "mail.example.com",
		Content:  "1.2.3.4",
		TTL:      300,
		Priority: &priority,
		Comment:  "mx record",
	})
	if err != nil {
		t.Fatalf("UpdateRecord failed: %v", err)
	}
	if rec.Priority == nil || *rec.Priority != 10 {
		t.Errorf("expected priority 10, got %v", rec.Priority)
	}
}

func TestCoverageLift_DNSPodCreateRecord_WithPriority(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"Response": {
				"RecordId": 12345,
				"RequestId": "req-priority"
			}
		}`))
	})

	priority := 20
	rec, err := d.CreateRecord(context.Background(), "42", Record{
		Type:     "MX",
		Name:     "mail",
		Content:  "mail.example.com",
		TTL:      600,
		Priority: &priority,
		Comment:  "remark",
	})
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}
	if rec.ID != "12345" {
		t.Errorf("expected record ID 12345, got %q", rec.ID)
	}
}

func TestCoverageLift_DNSPodUpdateRecord_WithTTLAndError(t *testing.T) {
	// 1. Success with TTL > 0 and Priority
	dSuccess := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"Response": {
				"RecordId": 12345,
				"RequestId": "req-update"
			}
		}`))
	})

	priority := 5
	rec, err := dSuccess.UpdateRecord(context.Background(), "42", Record{
		ID:       "12345",
		Type:     "MX",
		Name:     "mail",
		Content:  "mail.example.com",
		TTL:      300,
		Priority: &priority,
		Comment:  "remark",
	})
	if err != nil {
		t.Fatalf("UpdateRecord failed: %v", err)
	}
	if rec.ID != "12345" {
		t.Errorf("expected record ID 12345, got %q", rec.ID)
	}

	// 2. Error case triggering line 188-189
	dErr := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Internal Server Error`))
	})

	_, err = dErr.UpdateRecord(context.Background(), "42", Record{
		ID:      "12345",
		Type:    "A",
		Name:    "www",
		Content: "1.1.1.1",
		TTL:     300,
	})
	if err == nil {
		t.Fatalf("expected error from failed ModifyRecord, got nil")
	}
}
