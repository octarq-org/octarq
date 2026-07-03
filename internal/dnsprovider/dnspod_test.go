package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

// newDNSPodServer spins up a test server that mimics the tencentcloud API endpoints
// and returns a DNSPod provider pointed at it.
func newDNSPodServer(t *testing.T, handler http.HandlerFunc) *DNSPod {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	credential := common.NewCredential("test-id", "test-key")
	cpf := profile.NewClientProfile()
	endpoint := srv.URL
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
	}
	cpf.HttpProfile.Endpoint = endpoint
	cpf.HttpProfile.Scheme = "HTTP"

	client, err := dnspod.NewClient(credential, "", cpf)
	if err != nil {
		t.Fatalf("failed to initialize dnspod client: %v", err)
	}
	return &DNSPod{client: client}
}

func TestDNSPodListRecords(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"Response": {
				"RecordList": [
					{
						"RecordId": 7,
						"Name": "www",
						"Type": "A",
						"Value": "1.2.3.4",
						"TTL": 600,
						"MX": 0,
						"Remark": "hi"
					}
				],
				"RequestId": "req-1"
			}
		}`))
	})
	recs, err := d.ListRecords(context.Background(), "42")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
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
		w.Write([]byte(`{
			"Response": {
				"RecordId": 99,
				"RequestId": "req-2"
			}
		}`))
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
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{
			"Response": {
				"Error": {
					"Code": "AuthFailure.SignatureFailure",
					"Message": "The provided credentials could not be validated."
				},
				"RequestId": "req-error"
			}
		}`))
	})
	if _, err := d.ListRecords(context.Background(), "42"); err == nil {
		t.Fatal("expected error for error response")
	}
}

func TestDNSPodVerifyZone(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"Response": {
				"DomainList": [
					{
						"DomainId": 42,
						"Name": "example.com"
					}
				],
				"RequestId": "req-3"
			}
		}`))
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
		w.Write([]byte(`{
			"Response": {
				"RequestId": "req-4"
			}
		}`))
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
