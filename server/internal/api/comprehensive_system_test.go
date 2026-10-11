package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func comprehensiveTestOverview(t *testing.T, srv http.Handler, cookies []*http.Cookie) {
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

func comprehensiveTestSettings(t *testing.T, srv http.Handler, cookies []*http.Cookie) {
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("get settings failed: got %d", rec.Code)
	}

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

	reqInst := httptest.NewRequest(http.MethodGet, "/api/instance-settings", nil)
	for _, c := range cookies {
		reqInst.AddCookie(c)
	}
	recInst := httptest.NewRecorder()
	srv.ServeHTTP(recInst, reqInst)
	if recInst.Code != http.StatusOK {
		t.Errorf("get instance settings failed: got %d (%s)", recInst.Code, recInst.Body.String())
	}

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

func comprehensiveTestSMTPSenders(t *testing.T, srv http.Handler, cookies []*http.Cookie) {
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

	reqList := httptest.NewRequest(http.MethodGet, "/api/smtp-senders", nil)
	for _, c := range cookies {
		reqList.AddCookie(c)
	}
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Errorf("list smtp senders failed: got %d", recList.Code)
	}

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

func comprehensiveTestNotificationChannels(t *testing.T, srv http.Handler, cookies []*http.Cookie) {
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

	reqList := httptest.NewRequest(http.MethodGet, "/api/notification-channels", nil)
	for _, c := range cookies {
		reqList.AddCookie(c)
	}
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Errorf("list notify channels failed: got %d", recList.Code)
	}

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
