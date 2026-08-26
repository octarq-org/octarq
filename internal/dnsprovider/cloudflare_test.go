package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudflare/cloudflare-go"
)

func newCloudflareServer(t *testing.T, handler http.HandlerFunc) *Cloudflare {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	// Retries must be disabled: the SDK otherwise backs off exponentially on
	// the 5xx responses the error-path tests rely on, costing tens of seconds.
	api, err := cloudflare.NewWithAPIToken("test-token", cloudflare.BaseURL(srv.URL), cloudflare.HTTPClient(srv.Client()), cloudflare.UsingRetryPolicy(0, 0, 0))
	if err != nil {
		t.Fatalf("failed to initialize cloudflare api: %v", err)
	}
	return &Cloudflare{api: api}
}

func TestCloudflareListZones(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com"}]}`))
	})

	zones, err := c.ListZones(context.Background())
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	if len(zones) != 1 || zones[0].ID != "z1" || zones[0].Name != "example.com" {
		t.Errorf("zones mismatch: %+v", zones)
	}
}

func TestCloudflareListRecords(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/z1/dns_records" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"success":true,"result":[{"id":"r1","type":"A","name":"www.example.com","content":"1.1.1.1","ttl":300}],"result_info":{"page":1,"per_page":100,"total_pages":1,"total_count":1}}`))
	})

	recs, err := c.ListRecords(context.Background(), "z1")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(recs) != 1 || recs[0].ID != "r1" || recs[0].Type != "A" || recs[0].Content != "1.1.1.1" {
		t.Errorf("records mismatch: %+v", recs)
	}
}

func TestCloudflareCreateRecord(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"result":{"id":"r2","type":"A","name":"test","content":"2.2.2.2","ttl":120}}`))
	})

	rec, err := c.CreateRecord(context.Background(), "z1", Record{
		Type: "A", Name: "test", Content: "2.2.2.2", TTL: 120,
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if rec.ID != "r2" || rec.Content != "2.2.2.2" {
		t.Errorf("record mismatch: %+v", rec)
	}
}

func TestCloudflareUpdateRecord(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q", r.Method)
		}
		w.Write([]byte(`{"success":true,"result":{"id":"r2","type":"A","name":"test","content":"3.3.3.3"}}`))
	})

	rec, err := c.UpdateRecord(context.Background(), "z1", Record{
		ID: "r2", Type: "A", Name: "test", Content: "3.3.3.3",
	})
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if rec.Content != "3.3.3.3" {
		t.Errorf("record mismatch: %+v", rec)
	}
}

func TestCloudflareDeleteRecord(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/zones/z1/dns_records/r2" {
			t.Errorf("method = %q, path = %q", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"success":true,"result":{"id":"r2"}}`))
	})

	err := c.DeleteRecord(context.Background(), "z1", "r2")
	if err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
}

func TestCloudflareVerifyZone(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"result":{"id":"z1","name":"verified.com"}}`))
	})

	name, err := c.VerifyZone(context.Background(), "z1")
	if err != nil {
		t.Fatalf("VerifyZone: %v", err)
	}
	if name != "verified.com" {
		t.Errorf("zone name = %q, want verified.com", name)
	}
}

func TestCloudflareFactoryRegistered(t *testing.T) {
	p, err := New("cloudflare", []byte(`{"apiToken":"test-token"}`))
	if err != nil {
		t.Fatalf("New(cloudflare): %v", err)
	}
	if p == nil {
		t.Fatal("nil provider")
		return
	}

	if _, err := New("cloudflare", []byte(`{}`)); err == nil {
		t.Fatal("expected error for empty token")
		return
	}
	// Unparseable credentials JSON fails the factory, not a later call.
	if _, err := New("cloudflare", []byte("not json")); err == nil {
		t.Fatal("expected error for malformed credentials JSON")
		return
	}
}

func TestCloudflareListRecordsWithPriorityAndProxied(t *testing.T) {
	// The response carries priority + proxied + comment so the conversion
	// branches that map them into the neutral Record are exercised.
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"result":[{"id":"r1","type":"MX","name":"mx.example.com","content":"mail.example.com","ttl":300,"priority":10,"proxied":true,"comment":"note"}],"result_info":{"page":1,"per_page":100,"total_pages":1}}`))
	})
	recs, err := c.ListRecords(context.Background(), "z1")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Priority == nil || *r.Priority != 10 {
		t.Errorf("priority = %v, want 10", r.Priority)
	}
	if !r.Proxied || r.Comment != "note" {
		t.Errorf("proxied/comment not mapped: %+v", r)
	}
}

func TestCloudflareListRecordsPagination(t *testing.T) {
	requests := 0
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			w.Write([]byte(`{"success":true,"result":[{"id":"p1","type":"A","name":"a.example.com","content":"1.1.1.1","ttl":1}],"result_info":{"page":1,"per_page":100,"total_pages":2}}`))
			return
		}
		w.Write([]byte(`{"success":true,"result":[{"id":"p2","type":"A","name":"b.example.com","content":"2.2.2.2","ttl":1}],"result_info":{"page":2,"per_page":100,"total_pages":2}}`))
	})
	recs, err := c.ListRecords(context.Background(), "z1")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(recs) != 2 || recs[0].ID != "p1" || recs[1].ID != "p2" {
		t.Fatalf("paginated records wrong: %+v (requests=%d)", recs, requests)
	}
}

func TestCloudflareRoundTripWithPriority(t *testing.T) {
	// Create and Update both carry priority into the request and map a
	// priority-bearing response back out.
	prio := 20
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"result":{"id":"r3","type":"MX","name":"mx.example.com","content":"mail.example.com","ttl":600,"priority":20,"proxied":false}}`))
	})
	created, err := c.CreateRecord(context.Background(), "z1", Record{
		Type: "MX", Name: "mx.example.com", Content: "mail.example.com", TTL: 600, Priority: &prio,
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if created.Priority == nil || *created.Priority != 20 {
		t.Errorf("created priority = %v, want 20", created.Priority)
	}
	updated, err := c.UpdateRecord(context.Background(), "z1", Record{
		ID: "r3", Type: "MX", Name: "mx.example.com", Content: "mail.example.com", TTL: 600,
	})
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if updated.Priority == nil || *updated.Priority != 20 {
		t.Errorf("updated priority = %v, want 20", updated.Priority)
	}
}

func TestCloudflareErrorPaths(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success":false,"errors":[{"code":0,"message":"boom"}]}`))
	})

	if _, err := c.ListZones(context.Background()); err == nil {
		t.Error("ListZones must error on 500")
	}
	if _, err := c.ListRecords(context.Background(), "z1"); err == nil {
		t.Error("ListRecords must error on 500")
	}
	if _, err := c.CreateRecord(context.Background(), "z1", Record{Type: "A", Name: "x", Content: "1.1.1.1"}); err == nil {
		t.Error("CreateRecord must error on 500")
	}
	if _, err := c.UpdateRecord(context.Background(), "z1", Record{ID: "r", Type: "A", Name: "x", Content: "1.1.1.1"}); err == nil {
		t.Error("UpdateRecord must error on 500")
	}
	if err := c.DeleteRecord(context.Background(), "z1", "r"); err == nil {
		t.Error("DeleteRecord must error on 500")
	}
	if _, err := c.VerifyZone(context.Background(), "z1"); err == nil {
		t.Error("VerifyZone must error on 500")
	}
}

func TestCloudflareCreateRecordEmptyResponse(t *testing.T) {
	c := newCloudflareServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"result":null}`))
	})
	rec, err := c.CreateRecord(context.Background(), "z1", Record{Type: "A", Name: "x", Content: "1.1.1.1"})
	if err != nil {
		t.Fatalf("CreateRecord with null result: %v", err)
	}
	if rec.ID != "" {
		t.Errorf("expected zero-value record for null result, got %+v", rec)
	}
}
