package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/dnsprovider"
)

type mockDDNSProvider struct {
	records map[string]dnsprovider.Record
}

func newMockDDNSProvider() *mockDDNSProvider {
	return &mockDDNSProvider{records: make(map[string]dnsprovider.Record)}
}

func (m *mockDDNSProvider) ListZones(ctx context.Context) ([]dnsprovider.Zone, error) {
	return []dnsprovider.Zone{{ID: "z1", Name: "example.com"}}, nil
}

func (m *mockDDNSProvider) ListRecords(ctx context.Context, zoneID string) ([]dnsprovider.Record, error) {
	var res []dnsprovider.Record
	for _, r := range m.records {
		res = append(res, r)
	}
	return res, nil
}

func (m *mockDDNSProvider) CreateRecord(ctx context.Context, zoneID string, r dnsprovider.Record) (dnsprovider.Record, error) {
	if r.ID == "" {
		r.ID = "rec_" + r.Name
	}
	m.records[r.ID] = r
	return r, nil
}

func (m *mockDDNSProvider) UpdateRecord(ctx context.Context, zoneID string, r dnsprovider.Record) (dnsprovider.Record, error) {
	m.records[r.ID] = r
	return r, nil
}

func (m *mockDDNSProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	delete(m.records, recordID)
	return nil
}

func (m *mockDDNSProvider) VerifyZone(ctx context.Context, zoneID string) (string, error) {
	return "example.com", nil
}

func TestDDNSTokenManagement(t *testing.T) {
	_, srv, db := newVerifyHarness(t)
	const orgID = uint(1)

	dom := Domain{
		OrgID:  orgID,
		Name:   "home.example.com",
		ZoneID: "z1",
	}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}

	createReqBody := map[string]any{
		"domainId":   dom.ID,
		"recordName": "home.example.com",
		"recordType": "A",
		"label":      "Home Router",
	}
	bodyBytes, _ := json.Marshal(createReqBody)
	req := httptest.NewRequest("POST", "/api/dns/ddns", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create DDNS token: want 201, got %d (body %s)", rec.Code, rec.Body.String())
	}

	var createResp struct {
		ID         uint   `json:"id"`
		DomainID   uint   `json:"domainId"`
		RecordName string `json:"recordName"`
		RecordType string `json:"recordType"`
		Label      string `json:"label"`
		Secret     string `json:"secret"`
		UpdateURL  string `json:"updateUrl"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to unmarshal create response: %v", err)
	}

	if createResp.ID == 0 || createResp.Secret == "" || !strings.Contains(createResp.UpdateURL, createResp.Secret) {
		t.Fatalf("invalid create response: %+v", createResp)
	}

	rec = do(srv, "GET", "/api/dns/ddns")
	if rec.Code != http.StatusOK {
		t.Fatalf("list DDNS tokens: want 200, got %d", rec.Code)
	}
	var listResp []DDNSToken
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to unmarshal list response: %v", err)
	}
	if len(listResp) != 1 || listResp[0].ID != createResp.ID {
		t.Fatalf("list DDNS tokens mismatch: %+v", listResp)
	}

	rec = do(srv, "DELETE", fmt.Sprintf("/api/dns/ddns/%d", createResp.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete DDNS token: want 200, got %d", rec.Code)
	}

	rec = do(srv, "GET", "/api/dns/ddns")
	var listAfter []DDNSToken
	json.Unmarshal(rec.Body.Bytes(), &listAfter)
	if len(listAfter) != 0 {
		t.Fatalf("token list should be empty after deletion, got %d", len(listAfter))
	}
}

func TestDDNSUpdateEndpoint(t *testing.T) {
	mockProv := newMockDDNSProvider()
	dnsprovider.Register("mock_ddns", func(creds []byte) (dnsprovider.Provider, error) {
		return mockProv, nil
	})

	_, srv, db := newVerifyHarness(t)
	const orgID = uint(1)

	acc := ProviderAccount{
		OrgID:  orgID,
		Name:   "Mock Provider",
		Type:   "mock_ddns",
		Config: "mock_config",
	}
	if err := db.Create(&acc).Error; err != nil {
		t.Fatalf("failed to create provider account: %v", err)
	}

	dom := Domain{
		OrgID:             orgID,
		Name:              "example.com",
		ProviderAccountID: acc.ID,
		ZoneID:            "z1",
	}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}

	createReqBody := map[string]any{
		"domainId":   dom.ID,
		"recordName": "home.example.com",
		"recordType": "A",
		"label":      "Router Token",
	}
	bodyBytes, _ := json.Marshal(createReqBody)
	req := httptest.NewRequest("POST", "/api/dns/ddns", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var createResp struct {
		ID     uint   `json:"id"`
		Secret string `json:"secret"`
	}
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	secret := createResp.Secret

	// A. Bad Auth Test
	rec = do(srv, "GET", "/api/dns/ddns/update?token=invalid_secret&ip=1.2.3.4")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "badauth" {
		t.Fatalf("bad token: want 200 'badauth', got %d %q", rec.Code, rec.Body.String())
	}

	// B. First Update: Create record (good 1.2.3.4)
	rec = do(srv, "GET", "/api/dns/ddns/update?token="+secret+"&ip=1.2.3.4")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "good 1.2.3.4" {
		t.Fatalf("first update: want 200 'good 1.2.3.4', got %d %q", rec.Code, rec.Body.String())
	}

	var tok DDNSToken
	db.First(&tok, createResp.ID)
	if tok.LastIP != "1.2.3.4" || tok.LastSeenAt == nil {
		t.Fatalf("token DB state not updated: %+v", tok)
		return
	}

	// C. Second Update: No change (nochg 1.2.3.4)
	rec = do(srv, "GET", "/api/dns/ddns/update?token="+secret+"&ip=1.2.3.4")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "nochg 1.2.3.4" {
		t.Fatalf("second update: want 200 'nochg 1.2.3.4', got %d %q", rec.Code, rec.Body.String())
	}

	// D. Third Update: IP changed (good 5.6.7.8)
	rec = do(srv, "GET", "/api/dns/ddns/update?token="+secret+"&ip=5.6.7.8")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "good 5.6.7.8" {
		t.Fatalf("third update: want 200 'good 5.6.7.8', got %d %q", rec.Code, rec.Body.String())
	}

	// E. POST Form Body Update
	formData := url.Values{}
	formData.Set("token", secret)
	formData.Set("ip", "9.10.11.12")
	postReq := httptest.NewRequest("POST", "/api/dns/ddns/update", strings.NewReader(formData.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()
	srv.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK || strings.TrimSpace(postRec.Body.String()) != "good 9.10.11.12" {
		t.Fatalf("POST update: want 200 'good 9.10.11.12', got %d %q", postRec.Code, postRec.Body.String())
	}

	// F. IP Fallback from RemoteAddr
	fallbackReq := httptest.NewRequest("GET", "/api/dns/ddns/update?token="+secret, nil)
	fallbackReq.RemoteAddr = "192.168.1.100:12345"
	fallbackRec := httptest.NewRecorder()
	srv.ServeHTTP(fallbackRec, fallbackReq)

	if fallbackRec.Code != http.StatusOK || strings.TrimSpace(fallbackRec.Body.String()) != "good 192.168.1.100" {
		t.Fatalf("IP fallback update: want 200 'good 192.168.1.100', got %d %q", fallbackRec.Code, fallbackRec.Body.String())
	}
}
