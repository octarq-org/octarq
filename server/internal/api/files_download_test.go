package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestFiles_Download_And_TempToken(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	tokenUser1, org1ID := createTestUserAndOrg(t, h)
	tokenUser2, _ := createTestUserAndOrg(t, h)

	tmpDir := t.TempDir()
	storage, err := plugin.NewLocalStorageService(tmpDir, []byte("test-secret-key"), "")
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	h.SetStorage(storage)

	// Create and store a test file in storage and DB
	ctx := context.Background()
	fileContent := []byte("secret-image-or-document-content")
	key, hash, size, _, err := storage.PutObjectByHash(ctx, bytes.NewReader(fileContent), ".png")
	if err != nil {
		t.Fatalf("put object: %v", err)
	}

	testFile := models.File{
		OrgID:       org1ID,
		Name:        "logo.png",
		Size:        size,
		ContentType: "image/png",
		MD5:         hash,
		Path:        key,
	}
	if err := h.db.Create(&testFile).Error; err != nil {
		t.Fatalf("create test file: %v", err)
	}

	// 1. Generate temp token via POST /api/files/temp-token
	tokenReqJSON := fmt.Sprintf(`{"file_id": %d, "expire_seconds": 600}`, testFile.ID)
	req := httptest.NewRequest("POST", "/api/files/temp-token", strings.NewReader(tokenReqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: tokenUser1})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for create temp token, got %d: %s", rec.Code, rec.Body.String())
	}

	var tempTokenResp TempTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tempTokenResp); err != nil {
		t.Fatalf("unmarshal temp token: %v", err)
	}
	token := tempTokenResp.Token
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	expectedURL := fmt.Sprintf("/api/files/%d/download?token=%s", testFile.ID, token)
	if tempTokenResp.DownloadURL != expectedURL {
		t.Errorf("expected download URL %s, got %s", expectedURL, tempTokenResp.DownloadURL)
	}

	// 2. Download via temporary token WITHOUT any session / JWT auth
	dlURL := fmt.Sprintf("/api/files/%d/download?token=%s", testFile.ID, token)
	req = httptest.NewRequest("GET", dlURL, nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for download with valid token, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected image/png, got %s", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "inline") {
		t.Errorf("expected inline disposition, got %s", rec.Header().Get("Content-Disposition"))
	}
	if !bytes.Equal(rec.Body.Bytes(), fileContent) {
		t.Fatal("downloaded content mismatch")
	}

	// 3. Download with attachment disposition override
	req = httptest.NewRequest("GET", dlURL+"&disposition=attachment", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for attachment download, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("expected attachment disposition, got %s", rec.Header().Get("Content-Disposition"))
	}

	// 4. Download with tampered token -> 401
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d/download?token=%s", testFile.ID, token+"bad"), nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for tampered token, got %d", rec.Code)
	}

	// 5. Download with token for different file ID -> 401
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d/download?token=%s", testFile.ID+1, token), nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for file ID mismatch, got %d", rec.Code)
	}

	// 6. Download without token and without session -> 401
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d/download", testFile.ID), nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated download without token, got %d", rec.Code)
	}

	// 7. Download with valid session (same org) -> 200
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d/download", testFile.ID), nil)
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: tokenUser1})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for session-authenticated download, got %d: %s", rec.Code, rec.Body.String())
	}

	// 8. Download with session from a DIFFERENT org -> 403 Forbidden
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d/download", testFile.ID), nil)
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: tokenUser2})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-org download attempt, got %d", rec.Code)
	}

	// 9. Generate token for non-existent file -> 404
	req = httptest.NewRequest("POST", "/api/files/temp-token", strings.NewReader(`{"file_id": 99999}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: tokenUser1})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent file token, got %d", rec.Code)
	}

	// 10. Generate token for file belonging to another org -> 404
	req = httptest.NewRequest("POST", "/api/files/temp-token", strings.NewReader(fmt.Sprintf(`{"file_id": %d}`, testFile.ID)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: tokenUser2})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-org token generation, got %d", rec.Code)
	}

	// 11. Generate token with file_id = 0 -> 400
	req = httptest.NewRequest("POST", "/api/files/temp-token", strings.NewReader(`{"file_id": 0}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: tokenUser1})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for file_id=0, got %d", rec.Code)
	}

	// 12. GET /api/files/:id metadata
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d", testFile.ID), nil)
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: tokenUser1})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for get file metadata, got %d: %s", rec.Code, rec.Body.String())
	}
	var getFile models.File
	if err := json.Unmarshal(rec.Body.Bytes(), &getFile); err != nil {
		t.Fatalf("unmarshal get file resp: %v", err)
	}
	if getFile.ID != testFile.ID {
		t.Errorf("expected file ID %d, got %d", testFile.ID, getFile.ID)
	}

	// 13. GET /api/files/:id for other org -> 404
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d", testFile.ID), nil)
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: tokenUser2})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-org get file metadata, got %d", rec.Code)
	}
}
