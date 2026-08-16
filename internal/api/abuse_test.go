package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestAbuseEndpoints(t *testing.T) {
	srv, _ := newTestHandler(t)

	cookies := sessionCookies(t, 1, 1)

	// 1. Submit abuse report (Public)
	body := `{"slug":"nonexistent","reason":"phishing","description":"Phishing site"}`
	req := httptest.NewRequest(http.MethodPost, "/abuse", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit abuse: got %d, want 201 (%s)", rec.Code, rec.Body.String())
	}

	var submitResp struct {
		Ok bool `json:"ok"`
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !submitResp.Ok || submitResp.ID == 0 {
		t.Errorf("unexpected submit response: %+v", submitResp)
	}

	// 2. Submit 5 more times to trigger the rate limiter (making a total of 6 calls)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/abuse", strings.NewReader(body))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if i < 4 {
			if rec.Code != http.StatusCreated {
				t.Errorf("submit abuse %d: got %d, want 201", i, rec.Code)
			}
		} else {
			// The 6th request (since 1 + 5 = 6) should be rate-limited
			if rec.Code != http.StatusTooManyRequests {
				t.Errorf("expected 429 for rate-limited request, got %d (%s)", rec.Code, rec.Body.String())
			}
		}
	}

	// 3. List abuse reports (Admin)
	reqList := httptest.NewRequest(http.MethodGet, "/api/abuse?status=open", nil)
	for _, c := range cookies {
		reqList.AddCookie(c)
	}
	recList := httptest.NewRecorder()
	srv.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("list abuse reports: got %d, want 200", recList.Code)
	}

	var reports []models.AbuseReport
	if err := json.Unmarshal(recList.Body.Bytes(), &reports); err != nil {
		t.Fatalf("unmarshal reports: %v", err)
	}
	if len(reports) == 0 {
		t.Error("expected at least one abuse report in the list")
	}

	// 4. Update abuse report status (Admin)
	reportID := reports[0].ID
	updateBody := `{"status":"reviewed"}`
	reqUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/abuse/%d", reportID), strings.NewReader(updateBody))
	reqUpdate.SetPathValue("id", strconv.FormatUint(uint64(reportID), 10))
	for _, c := range cookies {
		reqUpdate.AddCookie(c)
	}
	recUpdate := httptest.NewRecorder()
	srv.ServeHTTP(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("update abuse report: got %d, want 200", recUpdate.Code)
	}

	var updatedReport models.AbuseReport
	if err := json.Unmarshal(recUpdate.Body.Bytes(), &updatedReport); err != nil {
		t.Fatalf("unmarshal updated report: %v", err)
	}
	if updatedReport.Status != "reviewed" {
		t.Errorf("expected status 'reviewed', got %q", updatedReport.Status)
	}
}

// A report is owned by the workspace that owns the reported link, and only that
// workspace's admins may change its status. Another workspace must get 404 and
// leave the report untouched — the status write is scoped through orgDB, not
// applied against the raw record.
func TestAbuseStatusUpdateIsOrgScoped(t *testing.T) {
	srv, db := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)
	org2 := sessionCookies(t, 2, 2)

	if rec := do(srv, http.MethodPost, "/api/links", org1, `{"slug":"sec-scope","target":"https://example.com"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create link: got %d — %s", rec.Code, rec.Body)
	}
	rec := do(srv, http.MethodPost, "/abuse", nil, `{"slug":"sec-scope","reason":"phishing","description":"scope test"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit abuse: got %d — %s", rec.Code, rec.Body)
	}
	var submitResp struct {
		OK bool `json:"ok"`
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("unmarshal submit response: %v", err)
	}
	path := fmt.Sprintf("/api/abuse/%d", submitResp.ID)

	if rec := do(srv, http.MethodPut, path, org2, `{"status":"dismissed"}`); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org abuse update: got %d, want 404 (%s)", rec.Code, rec.Body)
	}
	var rep models.AbuseReport
	if err := db.First(&rep, submitResp.ID).Error; err != nil {
		t.Fatalf("reload report: %v", err)
	}
	if rep.Status != "open" {
		t.Errorf("org2 changed org1's report status to %q", rep.Status)
	}

	if rec := do(srv, http.MethodPut, path, org1, `{"status":"reviewed"}`); rec.Code != http.StatusOK {
		t.Errorf("self abuse update: got %d, want 200 (%s)", rec.Code, rec.Body)
	}
	if err := db.First(&rep, submitResp.ID).Error; err != nil {
		t.Fatalf("reload report: %v", err)
	}
	if rep.Status != "reviewed" {
		t.Errorf("self abuse update did not persist: got %q, want reviewed", rep.Status)
	}
}
