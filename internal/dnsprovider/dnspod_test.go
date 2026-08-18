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
	endpoint := strings.TrimPrefix(srv.URL, "http://")
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
	p, err := New("dnspod", []byte(`{"secretId":"id","secretKey":"key"}`))
	if err != nil {
		t.Fatalf("New(dnspod): %v", err)
	}
	if p == nil {
		t.Fatal("nil provider")
	}
	// Missing creds fails.
	if _, err := New("dnspod", []byte(`{}`)); err == nil {
		t.Fatal("expected error for empty creds")
	}
	// Malformed JSON fails the factory.
	if _, err := New("dnspod", []byte("not json")); err == nil {
		t.Fatal("expected error for malformed credentials JSON")
	}
}

// dnspodActionHandler dispatches the response per the SDK's X-TC-Action header
// so one server can serve every operation.
func dnspodActionHandler(respond func(action string, w http.ResponseWriter) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if respond(r.Header.Get("X-TC-Action"), w) {
			return
		}
		http.Error(w, `{"Response":{}}`, http.StatusBadRequest)
	}
}

func TestDNSPodListZones(t *testing.T) {
	d := newDNSPodServer(t, dnspodActionHandler(func(action string, w http.ResponseWriter) bool {
		if action != "DescribeDomainList" {
			return false
		}
		w.Write([]byte(`{"Response":{"DomainList":[
			{"DomainId": 42, "Name": "example.com"},
			{"DomainId": null, "Name": null}
		],"RequestId":"r1"}}`))
		return true
	}))
	zones, err := d.ListZones(context.Background())
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	if len(zones) != 1 || zones[0].ID != "42" || zones[0].Name != "example.com" {
		t.Errorf("zones = %+v, want a single [42 example.com]", zones)
	}
}

func TestDNSPodListZonesError(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := d.ListZones(context.Background()); err == nil {
		t.Error("ListZones must error on 500")
	}
}

func TestDNSPodListRecordsVariants(t *testing.T) {
	// Records with nil-ish fields are skipped; a record with an MX>0 priority
	// maps it; a nil TTL falls back to the default.
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Response":{"RecordList":[
			{"RecordId": 1, "Type": "A", "Name": "www", "Value": "1.1.1.1", "TTL": 60, "MX": 0, "Remark": "ok"},
			{"RecordId": 2, "Type": "MX", "Name": "mx", "Value": "mail.example.com", "MX": 10, "Remark": null},
			{"RecordId": 3, "Type": null, "Name": null, "Value": null}
		],"RequestId":"r2"}}`))
	})
	recs, err := d.ListRecords(context.Background(), "42")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 usable records, got %d: %+v", len(recs), recs)
	}
	if recs[0].TTL != 60 {
		t.Errorf("record 1 ttl = %d, want 60", recs[0].TTL)
	}
	if recs[1].Priority == nil || *recs[1].Priority != 10 {
		t.Errorf("record 2 priority = %v, want 10 (MX=10)", recs[1].Priority)
	}
	if recs[1].Comment != "" {
		t.Errorf("record 2 comment = %q, want empty for null Remark", recs[1].Comment)
	}
	// A nil MX with a non-nil TTL keeps the priority nil and uses the TTL.
	if recs[0].Priority != nil {
		t.Errorf("record 1 priority = %v, want nil for MX=0", recs[0].Priority)
	}
}

func TestDNSPodGetPtrStringNil(t *testing.T) {
	if got := getPtrString(nil); got != "" {
		t.Errorf("getPtrString(nil) = %q, want empty", got)
	}
}

func TestDNSPodInvalidZoneIDs(t *testing.T) {
	d := newDNSPodServer(t, dnspodActionHandler(func(action string, w http.ResponseWriter) bool {
		return false
	}))
	if _, err := d.ListRecords(context.Background(), "not-a-number"); err == nil {
		t.Error("ListRecords with invalid zoneID must error")
	}
	if _, err := d.CreateRecord(context.Background(), "not-a-number", Record{Type: "A"}); err == nil {
		t.Error("CreateRecord with invalid zoneID must error")
	}
	if _, err := d.UpdateRecord(context.Background(), "not-a-number", Record{ID: "1"}); err == nil {
		t.Error("UpdateRecord with invalid zoneID must error")
	}
	if err := d.DeleteRecord(context.Background(), "not-a-number", "1"); err == nil {
		t.Error("DeleteRecord with invalid zoneID must error")
	}
	if _, err := d.VerifyZone(context.Background(), "not-a-number"); err == nil {
		t.Error("VerifyZone with invalid zoneID must error")
	}
	// DeleteRecord with an invalid record id errors after the zone id parsed.
	if err := d.DeleteRecord(context.Background(), "42", "not-a-number"); err == nil {
		t.Error("DeleteRecord with invalid record id must error")
	}
	// UpdateRecord with an invalid record id errors after the zone id parsed.
	if _, err := d.UpdateRecord(context.Background(), "42", Record{ID: "not-a-number"}); err == nil {
		t.Error("UpdateRecord with invalid record id must error")
	}
}

func TestDNSPodUpdateRecordFlow(t *testing.T) {
	d := newDNSPodServer(t, dnspodActionHandler(func(action string, w http.ResponseWriter) bool {
		if action != "ModifyRecord" {
			return false
		}
		w.Write([]byte(`{"Response":{"RequestId":"r3"}}`))
		return true
	}))
	prio := 5
	out, err := d.UpdateRecord(context.Background(), "42", Record{
		ID: "99", Type: "MX", Name: "mx.example.com", Content: "mail.example.com", TTL: 0, Priority: &prio, Comment: "c",
	})
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if out.ID != "99" {
		t.Errorf("UpdateRecord id = %q, want 99", out.ID)
	}
}

func TestDNSPodCreateRecordPaths(t *testing.T) {
	// Priority and comment flow into the request; an empty response body is
	// rejected rather than silently turning into a record.
	d := newDNSPodServer(t, dnspodActionHandler(func(action string, w http.ResponseWriter) bool {
		if action != "CreateRecord" {
			return false
		}
		http.Error(w, "", http.StatusInternalServerError)
		return true
	}))
	if _, err := d.CreateRecord(context.Background(), "42", Record{Type: "A", Name: "www", Content: "1.1.1.1"}); err == nil {
		t.Error("CreateRecord must error on 500")
	}

	d2 := newDNSPodServer(t, dnspodActionHandler(func(action string, w http.ResponseWriter) bool {
		if action != "CreateRecord" {
			return false
		}
		w.Write([]byte(`{"Response":{"RequestId":"r4"}}`)) // no RecordId
		return true
	}))
	if _, err := d2.CreateRecord(context.Background(), "42", Record{Type: "A", Name: "www", Content: "1.1.1.1"}); err == nil {
		t.Error("CreateRecord must error when the response has no RecordId")
	}
}

func TestDNSPodVerifyZoneNotFound(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Response":{"DomainList":[{"DomainId": 1, "Name": "other.com"}],"RequestId":"r5"}}`))
	})
	if _, err := d.VerifyZone(context.Background(), "42"); err == nil {
		t.Error("VerifyZone must error when the zone id is absent from the list")
	}

	// A 500 from the API surfaces as an error too.
	d2 := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := d2.VerifyZone(context.Background(), "42"); err == nil {
		t.Error("VerifyZone must error on 500")
	}
}

func TestDNSPodDeleteRecordError(t *testing.T) {
	d := newDNSPodServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := d.DeleteRecord(context.Background(), "42", "99"); err == nil {
		t.Error("DeleteRecord must error on 500")
	}
}
