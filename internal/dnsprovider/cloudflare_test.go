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
	api, err := cloudflare.NewWithAPIToken("test-token", cloudflare.BaseURL(srv.URL), cloudflare.HTTPClient(srv.Client()))
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
	}

	if _, err := New("cloudflare", []byte(`{}`)); err == nil {
		t.Fatal("expected error for empty token")
	}
}
