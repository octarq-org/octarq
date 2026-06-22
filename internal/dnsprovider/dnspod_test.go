package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newDNSPodServer spins up a test server that mimics the dnsapi.cn endpoints and
// returns a DNSPod provider pointed at it.
func newDNSPodServer(t *testing.T, handler http.HandlerFunc) *DNSPod {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &DNSPod{loginToken: "1,abc", base: srv.URL, hc: srv.Client()}
}

func TestDNSPodListRecords(t *testing.T) {
	var gotPath string
	var gotLoginToken string
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotLoginToken = r.PostFormValue("login_token")
		w.Write([]byte(`{"status":{"code":"1","message":"ok"},
			"records":[{"id":"7","name":"www","type":"A","value":"1.2.3.4","ttl":"600","mx":"0","remark":"hi"}]}`))
	})
	recs, err := d.ListRecords(context.Background(), "42")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if gotPath != "/Record.List" {
		t.Errorf("path = %q want /Record.List", gotPath)
	}
	if gotLoginToken != "1,abc" {
		t.Errorf("login_token = %q want 1,abc", gotLoginToken)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.ID != "7" || r.Type != "A" || r.Content != "1.2.3.4" || r.TTL != 600 {
		t.Errorf("record mapping wrong: %+v", r)
	}
	if r.Comment != "hi" {
		t.Errorf("remark not mapped to comment: %q", r.Comment)
	}
}

func TestDNSPodCreateRecord(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostFormValue("domain_id") != "42" {
			t.Errorf("domain_id = %q", r.PostFormValue("domain_id"))
		}
		if r.PostFormValue("record_type") != "A" {
			t.Errorf("record_type = %q", r.PostFormValue("record_type"))
		}
		if r.PostFormValue("remark") != "my note" {
			t.Errorf("remark = %q", r.PostFormValue("remark"))
		}
		w.Write([]byte(`{"status":{"code":"1","message":"ok"},"record":{"id":99,"name":"www","value":"1.2.3.4","status":"enable"}}`))
	})
	out, err := d.CreateRecord(context.Background(), "42", Record{
		Type: "A", Name: "www", Content: "1.2.3.4", TTL: 600, Comment: "my note",
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if out.ID != "99" {
		t.Errorf("created id = %q want 99", out.ID)
	}
}

func TestDNSPodErrorStatus(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":{"code":"6","message":"login error"}}`))
	})
	if _, err := d.ListRecords(context.Background(), "42"); err == nil {
		t.Fatal("expected error for non-success status code")
	}
}

func TestDNSPodVerifyZone(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Domain.Info" {
			t.Errorf("path = %q want /Domain.Info", r.URL.Path)
		}
		w.Write([]byte(`{"status":{"code":"1","message":"ok"},"domain":{"name":"example.com"}}`))
	})
	name, err := d.VerifyZone(context.Background(), "42")
	if err != nil {
		t.Fatalf("VerifyZone: %v", err)
	}
	if name != "example.com" {
		t.Errorf("name = %q want example.com", name)
	}
}

func TestDNSPodDeleteRecord(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostFormValue("record_id") != "99" {
			t.Errorf("record_id = %q", r.PostFormValue("record_id"))
		}
		w.Write([]byte(`{"status":{"code":"1","message":"ok"}}`))
	})
	if err := d.DeleteRecord(context.Background(), "42", "99"); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
}

func TestDNSPodFactoryRegistered(t *testing.T) {
	p, err := New("dnspod", []byte(`{"token":"123,abctoken"}`))
	if err != nil {
		t.Fatalf("New(dnspod): %v", err)
	}
	if p == nil {
		t.Fatal("nil provider")
	}
	// secretId/secretKey form also works.
	if _, err := New("dnspod", []byte(`{"secretId":"id","secretKey":"key"}`)); err != nil {
		t.Fatalf("New(dnspod) split form: %v", err)
	}
	// Missing creds fails.
	if _, err := New("dnspod", []byte(`{}`)); err == nil {
		t.Fatal("expected error for empty creds")
	}
}
