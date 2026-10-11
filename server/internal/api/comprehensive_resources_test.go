package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/dnsprovider"
)

type mockProvider struct{}

func (mockProvider) ListZones(ctx context.Context) ([]dnsprovider.Zone, error) {
	return []dnsprovider.Zone{{ID: "z123", Name: "mockdomain.com"}}, nil
}
func (mockProvider) ListRecords(ctx context.Context, zoneID string) ([]dnsprovider.Record, error) {
	return []dnsprovider.Record{{ID: "r123", Type: "A", Name: "www.mockdomain.com", Content: "1.2.3.4"}}, nil
}
func (mockProvider) CreateRecord(ctx context.Context, zoneID string, r dnsprovider.Record) (dnsprovider.Record, error) {
	r.ID = "r123"
	return r, nil
}
func (mockProvider) UpdateRecord(ctx context.Context, zoneID string, r dnsprovider.Record) (dnsprovider.Record, error) {
	return r, nil
}
func (mockProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	return nil
}
func (mockProvider) VerifyZone(ctx context.Context, zoneID string) (string, error) {
	return "mockdomain.com", nil
}

func init() {
	dnsprovider.Register("mock", func(credsJSON []byte) (dnsprovider.Provider, error) {
		return mockProvider{}, nil
	})
}

func comprehensiveTestProviderAccounts(t *testing.T, srv http.Handler, cookies []*http.Cookie) uint {
	body := `{"name":"mock-acc","type":"mock","config":{"token":"test-token"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/provider-accounts", strings.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create provider acc failed: got %d (%s)", rec.Code, rec.Body.String())
	}
	var acc struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &acc)

	reqList := httptest.NewRequest(http.MethodGet, "/api/provider-accounts", nil)
	for _, c := range cookies {
		reqList.AddCookie(c)
	}
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Errorf("list provider accounts failed: got %d", recList.Code)
	}

	bodyUpdate := `{"name":"cf-acc-updated"}`
	reqUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/provider-accounts/%d", acc.ID), strings.NewReader(bodyUpdate))
	reqUpdate.SetPathValue("id", fmt.Sprintf("%d", acc.ID))
	for _, c := range cookies {
		reqUpdate.AddCookie(c)
	}
	recUpdate := httptest.NewRecorder()
	srv.ServeHTTP(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Errorf("update provider acc failed: got %d (%s)", recUpdate.Code, recUpdate.Body.String())
	}
	return acc.ID
}

func comprehensiveTestDomainsAndRecords(t *testing.T, srv http.Handler, cookies []*http.Cookie, providerAccID uint) {
	bodySync := fmt.Sprintf(`{"providerAccountId":%d}`, providerAccID)
	reqSync := httptest.NewRequest(http.MethodPost, "/api/domains/sync", strings.NewReader(bodySync))
	for _, c := range cookies {
		reqSync.AddCookie(c)
	}
	recSync := httptest.NewRecorder()
	srv.ServeHTTP(recSync, reqSync)
	if recSync.Code != http.StatusOK {
		t.Fatalf("sync domains failed: got %d (%s)", recSync.Code, recSync.Body.String())
	}

	reqList := httptest.NewRequest(http.MethodGet, "/api/domains", nil)
	for _, c := range cookies {
		reqList.AddCookie(c)
	}
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("list domains failed: got %d", recList.Code)
	}
	var domainsList []struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(recList.Body.Bytes(), &domainsList)
	if len(domainsList) == 0 {
		t.Fatalf("expected synced domain in list, got empty")
	}
	domainID := domainsList[0].ID

	bodyUpdate := fmt.Sprintf(`{"note":"updated domain note","forLink":true,"forMail":true,"zoneId":"z123","providerAccountId":%d}`, providerAccID)
	reqUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/domains/%d", domainID), strings.NewReader(bodyUpdate))
	reqUpdate.SetPathValue("id", fmt.Sprintf("%d", domainID))
	for _, c := range cookies {
		reqUpdate.AddCookie(c)
	}
	recUpdate := httptest.NewRecorder()
	srv.ServeHTTP(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Errorf("update domain failed: got %d (%s)", recUpdate.Code, recUpdate.Body.String())
	}

	bodyRec := `{"type":"A","name":"www","content":"1.2.3.4","ttl":3600}`
	reqRec := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/domains/%d/records", domainID), strings.NewReader(bodyRec))
	reqRec.SetPathValue("id", fmt.Sprintf("%d", domainID))
	for _, c := range cookies {
		reqRec.AddCookie(c)
	}
	recRec := httptest.NewRecorder()
	srv.ServeHTTP(recRec, reqRec)
	if recRec.Code != http.StatusCreated {
		t.Errorf("create dns record failed: got %d (%s)", recRec.Code, recRec.Body.String())
	}

	reqRecList := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/domains/%d/records", domainID), nil)
	reqRecList.SetPathValue("id", fmt.Sprintf("%d", domainID))
	for _, c := range cookies {
		reqRecList.AddCookie(c)
	}
	recRecList := httptest.NewRecorder()
	srv.ServeHTTP(recRecList, reqRecList)
	if recRecList.Code != http.StatusOK {
		t.Errorf("list dns records failed: got %d", recRecList.Code)
	}

	bodyRecUpd := `{"type":"A","name":"www","content":"5.6.7.8","ttl":1800}`
	reqRecUpd := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/domains/%d/records/r123", domainID), strings.NewReader(bodyRecUpd))
	reqRecUpd.SetPathValue("id", fmt.Sprintf("%d", domainID))
	reqRecUpd.SetPathValue("rid", "r123")
	for _, c := range cookies {
		reqRecUpd.AddCookie(c)
	}
	recRecUpd := httptest.NewRecorder()
	srv.ServeHTTP(recRecUpd, reqRecUpd)
	if recRecUpd.Code != http.StatusOK {
		t.Errorf("update dns record failed: got %d (%s)", recRecUpd.Code, recRecUpd.Body.String())
	}

	reqRecDel := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/domains/%d/records/r123", domainID), nil)
	reqRecDel.SetPathValue("id", fmt.Sprintf("%d", domainID))
	reqRecDel.SetPathValue("rid", "r123")
	for _, c := range cookies {
		reqRecDel.AddCookie(c)
	}
	recRecDel := httptest.NewRecorder()
	srv.ServeHTTP(recRecDel, reqRecDel)
	if recRecDel.Code != http.StatusOK {
		t.Errorf("delete dns record failed: got %d", recRecDel.Code)
	}

	reqDelete := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/domains/%d", domainID), nil)
	reqDelete.SetPathValue("id", fmt.Sprintf("%d", domainID))
	for _, c := range cookies {
		reqDelete.AddCookie(c)
	}
	recDelete := httptest.NewRecorder()
	srv.ServeHTTP(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Errorf("delete domain failed: got %d", recDelete.Code)
	}
}

func comprehensiveTestMailboxes(t *testing.T, srv http.Handler, cookies []*http.Cookie) {
	body := `{"address":"support@example.com","note":"customer support","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/mailboxes", strings.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create mailbox failed: got %d (%s)", rec.Code, rec.Body.String())
	}
	var mb struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &mb)

	reqList := httptest.NewRequest(http.MethodGet, "/api/mailboxes", nil)
	for _, c := range cookies {
		reqList.AddCookie(c)
	}
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Errorf("list mailboxes failed: got %d", recList.Code)
	}

	bodyUpdate := `{"note":"updated support note"}`
	reqUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/mailboxes/%d", mb.ID), strings.NewReader(bodyUpdate))
	reqUpdate.SetPathValue("id", fmt.Sprintf("%d", mb.ID))
	for _, c := range cookies {
		reqUpdate.AddCookie(c)
	}
	recUpdate := httptest.NewRecorder()
	srv.ServeHTTP(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Errorf("update mailbox failed: got %d (%s)", recUpdate.Code, recUpdate.Body.String())
	}

	reqDelete := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/mailboxes/%d", mb.ID), nil)
	reqDelete.SetPathValue("id", fmt.Sprintf("%d", mb.ID))
	for _, c := range cookies {
		reqDelete.AddCookie(c)
	}
	recDelete := httptest.NewRecorder()
	srv.ServeHTTP(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Errorf("delete mailbox failed: got %d", recDelete.Code)
	}
}

func comprehensiveTestLinks(t *testing.T, srv http.Handler, cookies []*http.Cookie) {
	body := `{"slug":"mylink","target":"https://mylink.com","note":"test link"}`
	req := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create link failed: got %d (%s)", rec.Code, rec.Body.String())
	}
	var link struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &link)

	reqList := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	for _, c := range cookies {
		reqList.AddCookie(c)
	}
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Errorf("list links failed: got %d", recList.Code)
	}

	reqGet := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/links/%d", link.ID), nil)
	reqGet.SetPathValue("id", fmt.Sprintf("%d", link.ID))
	for _, c := range cookies {
		reqGet.AddCookie(c)
	}
	recGet := httptest.NewRecorder()
	srv.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Errorf("get link failed: got %d", recGet.Code)
	}

	bodyUpdate := `{"target":"https://mylink-updated.com"}`
	reqUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/links/%d", link.ID), strings.NewReader(bodyUpdate))
	reqUpdate.SetPathValue("id", fmt.Sprintf("%d", link.ID))
	for _, c := range cookies {
		reqUpdate.AddCookie(c)
	}
	recUpdate := httptest.NewRecorder()
	srv.ServeHTTP(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Errorf("update link failed: got %d (%s)", recUpdate.Code, recUpdate.Body.String())
	}

	reqDelete := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/links/%d", link.ID), nil)
	reqDelete.SetPathValue("id", fmt.Sprintf("%d", link.ID))
	for _, c := range cookies {
		reqDelete.AddCookie(c)
	}
	recDelete := httptest.NewRecorder()
	srv.ServeHTTP(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Errorf("delete link failed: got %d", recDelete.Code)
	}
}

func comprehensiveCleanupProviderAccount(t *testing.T, srv http.Handler, cookies []*http.Cookie, providerAccID uint) {
	reqDelete := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/provider-accounts/%d", providerAccID), nil)
	reqDelete.SetPathValue("id", strconv.FormatUint(uint64(providerAccID), 10))
	for _, c := range cookies {
		reqDelete.AddCookie(c)
	}
	recDelete := httptest.NewRecorder()
	srv.ServeHTTP(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Errorf("cleanup provider acc failed: got %d", recDelete.Code)
	}
}
