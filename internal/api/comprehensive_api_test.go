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
	"time"

	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
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

func TestComprehensiveAPI(t *testing.T) {
	srv, db := newTestHandler(t)

	cookies := sessionCookies(t, 1, 1)

	// Seed the caller as an org owner so role-gated endpoints (e.g. settings)
	// behave as they do after a real admin login.
	db.Create(&models.OrgMember{OrgID: 1, UserID: 1, Role: "owner"})

	// 1. Overview API
	{
		req := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("overview failed: got %d", rec.Code)
		}
	}

	// 2. Settings API
	{
		// GET settings
		req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("get settings failed: got %d", rec.Code)
		}

		// PUT settings (workspace)
		body := `{"catchAll":true}`
		reqPut := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
		for _, c := range cookies {
			reqPut.AddCookie(c)
		}
		recPut := httptest.NewRecorder()
		srv.ServeHTTP(recPut, reqPut)
		if recPut.Code != http.StatusOK {
			t.Errorf("put settings failed: got %d (%s)", recPut.Code, recPut.Body.String())
		}

		// GET instance settings
		reqInst := httptest.NewRequest(http.MethodGet, "/api/instance-settings", nil)
		for _, c := range cookies {
			reqInst.AddCookie(c)
		}
		recInst := httptest.NewRecorder()
		srv.ServeHTTP(recInst, reqInst)
		if recInst.Code != http.StatusOK {
			t.Errorf("get instance settings failed: got %d (%s)", recInst.Code, recInst.Body.String())
		}

		// PUT instance settings
		bodyInst := `{"dataRetentionDays":30}`
		reqPutInst := httptest.NewRequest(http.MethodPut, "/api/instance-settings", strings.NewReader(bodyInst))
		for _, c := range cookies {
			reqPutInst.AddCookie(c)
		}
		recPutInst := httptest.NewRecorder()
		srv.ServeHTTP(recPutInst, reqPutInst)
		if recPutInst.Code != http.StatusOK {
			t.Errorf("put instance settings failed: got %d (%s)", recPutInst.Code, recPutInst.Body.String())
		}

		// 2.5 MCP API tests
		// GET /api/mcp/sse without auth -> 401
		reqMcpUnauth := httptest.NewRequest(http.MethodGet, "/api/mcp/sse", nil)
		recMcpUnauth := httptest.NewRecorder()
		srv.ServeHTTP(recMcpUnauth, reqMcpUnauth)
		if recMcpUnauth.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for unauth mcp, got %d", recMcpUnauth.Code)
		}

		// GET /api/mcp/sse with auth (cancel via context to avoid blocking)
		ctxMcp, cancelMcp := context.WithCancel(context.Background())
		reqMcpAuth := httptest.NewRequest(http.MethodGet, "/api/mcp/sse", nil).WithContext(ctxMcp)
		for _, c := range cookies {
			reqMcpAuth.AddCookie(c)
		}
		recMcpAuth := httptest.NewRecorder()
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancelMcp()
		}()
		srv.ServeHTTP(recMcpAuth, reqMcpAuth)
		if recMcpAuth.Code != http.StatusOK {
			t.Errorf("expected 200 for auth mcp sse, got %d (%s)", recMcpAuth.Code, recMcpAuth.Body.String())
		}
	}

	// 3. SMTP Senders CRUD
	{
		// Create SMTPSender
		body := `{"name":"test-sender","host":"smtp.example.com","port":587,"user":"user","pass":"pass","fromEmail":"test@example.com"}`
		req := httptest.NewRequest(http.MethodPost, "/api/smtp-senders", strings.NewReader(body))
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create smtp sender failed: got %d (%s)", rec.Code, rec.Body.String())
		}
		var sender struct {
			ID uint `json:"id"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &sender)

		// List SMTP Senders
		reqList := httptest.NewRequest(http.MethodGet, "/api/smtp-senders", nil)
		for _, c := range cookies {
			reqList.AddCookie(c)
		}
		recList := httptest.NewRecorder()
		srv.ServeHTTP(recList, reqList)
		if recList.Code != http.StatusOK {
			t.Errorf("list smtp senders failed: got %d", recList.Code)
		}

		// Update SMTP Sender
		bodyUpdate := `{"name":"test-sender-updated"}`
		reqUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/smtp-senders/%d", sender.ID), strings.NewReader(bodyUpdate))
		reqUpdate.SetPathValue("id", fmt.Sprintf("%d", sender.ID))
		for _, c := range cookies {
			reqUpdate.AddCookie(c)
		}
		recUpdate := httptest.NewRecorder()
		srv.ServeHTTP(recUpdate, reqUpdate)
		if recUpdate.Code != http.StatusOK {
			t.Errorf("update smtp sender failed: got %d (%s)", recUpdate.Code, recUpdate.Body.String())
		}

		// Delete SMTP Sender
		reqDelete := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/smtp-senders/%d", sender.ID), nil)
		reqDelete.SetPathValue("id", fmt.Sprintf("%d", sender.ID))
		for _, c := range cookies {
			reqDelete.AddCookie(c)
		}
		recDelete := httptest.NewRecorder()
		srv.ServeHTTP(recDelete, reqDelete)
		if recDelete.Code != http.StatusOK {
			t.Errorf("delete smtp sender failed: got %d", recDelete.Code)
		}
	}

	// 4. Provider Accounts CRUD
	var providerAccID uint
	{
		// Create Provider Account
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
		providerAccID = acc.ID

		// List Provider Accounts
		reqList := httptest.NewRequest(http.MethodGet, "/api/provider-accounts", nil)
		for _, c := range cookies {
			reqList.AddCookie(c)
		}
		recList := httptest.NewRecorder()
		srv.ServeHTTP(recList, reqList)
		if recList.Code != http.StatusOK {
			t.Errorf("list provider accounts failed: got %d", recList.Code)
		}

		// Update Provider Account
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
	}

	// 5. Notification Channels CRUD
	{
		// Create Notification Channel
		body := `{"name":"my-channel","type":"webhook","config":"{\"url\":\"http://localhost\"}","enabled":true}`
		req := httptest.NewRequest(http.MethodPost, "/api/notification-channels", strings.NewReader(body))
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create notify ch failed: got %d (%s)", rec.Code, rec.Body.String())
		}
		var ch struct {
			ID uint `json:"id"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &ch)

		// List Notification Channels
		reqList := httptest.NewRequest(http.MethodGet, "/api/notification-channels", nil)
		for _, c := range cookies {
			reqList.AddCookie(c)
		}
		recList := httptest.NewRecorder()
		srv.ServeHTTP(recList, reqList)
		if recList.Code != http.StatusOK {
			t.Errorf("list notify channels failed: got %d", recList.Code)
		}

		// Update Notification Channel
		bodyUpdate := `{"enabled":false}`
		reqUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/notification-channels/%d", ch.ID), strings.NewReader(bodyUpdate))
		reqUpdate.SetPathValue("id", fmt.Sprintf("%d", ch.ID))
		for _, c := range cookies {
			reqUpdate.AddCookie(c)
		}
		recUpdate := httptest.NewRecorder()
		srv.ServeHTTP(recUpdate, reqUpdate)
		if recUpdate.Code != http.StatusOK {
			t.Errorf("update notify ch failed: got %d (%s)", recUpdate.Code, recUpdate.Body.String())
		}

		// Delete Notification Channel
		reqDelete := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/notification-channels/%d", ch.ID), nil)
		reqDelete.SetPathValue("id", fmt.Sprintf("%d", ch.ID))
		for _, c := range cookies {
			reqDelete.AddCookie(c)
		}
		recDelete := httptest.NewRecorder()
		srv.ServeHTTP(recDelete, reqDelete)
		if recDelete.Code != http.StatusOK {
			t.Errorf("delete notify ch failed: got %d", recDelete.Code)
		}
	}

	// 6. Domains & Records CRUD
	{
		// Sync domains from Mock provider
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

		// List Domains to find the synced domain ID
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

		// Update Domain
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

		// Create Record
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

		// List Records
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

		// Update Record
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

		// Delete Record
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

		// Delete Domain
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

	// 7. Mailboxes CRUD
	{
		// Create Mailbox
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

		// List Mailboxes
		reqList := httptest.NewRequest(http.MethodGet, "/api/mailboxes", nil)
		for _, c := range cookies {
			reqList.AddCookie(c)
		}
		recList := httptest.NewRecorder()
		srv.ServeHTTP(recList, reqList)
		if recList.Code != http.StatusOK {
			t.Errorf("list mailboxes failed: got %d", recList.Code)
		}

		// Update Mailbox
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

		// Delete Mailbox
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

	// 8. Links CRUD
	{
		// Create Link
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

		// List Links
		reqList := httptest.NewRequest(http.MethodGet, "/api/links", nil)
		for _, c := range cookies {
			reqList.AddCookie(c)
		}
		recList := httptest.NewRecorder()
		srv.ServeHTTP(recList, reqList)
		if recList.Code != http.StatusOK {
			t.Errorf("list links failed: got %d", recList.Code)
		}

		// Get Link
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

		// Update Link
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

		// Delete Link
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

	// 9. Clean up provider account we left
	{
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

	// 10. Inbound Email Webhook — /api/webhook/{orgSlug}/email/inbound/{token}
	{
		// The tenant org owns the inbound token (in its slug'd path) and the mailbox.
		org := models.Org{Name: "Acme", Slug: "acme", InboundToken: "my-inbound-token"}
		db.Create(&org)
		db.Create(&models.Mailbox{
			OrgID:   org.ID,
			Address: "support@example.com",
			Enabled: true,
		})

		body := "From: alice@example.com\r\nTo: support@example.com\r\nSubject: Help\r\n\r\nHello Support"
		req := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/my-inbound-token", strings.NewReader(body))
		req.Header.Set("X-Octarq-To", "support@example.com")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("inbound webhook failed: got %d (%s)", rec.Code, rec.Body.String())
		}

		// Wrong token → 401. Unknown org slug → 404.
		bad := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/inbound/nope", strings.NewReader(body))
		bad.Header.Set("X-Octarq-To", "support@example.com")
		badRec := httptest.NewRecorder()
		srv.ServeHTTP(badRec, bad)
		if badRec.Code != http.StatusUnauthorized {
			t.Errorf("bad token: got %d, want 401", badRec.Code)
		}
	}
}
