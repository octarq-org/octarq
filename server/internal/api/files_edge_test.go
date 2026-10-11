package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestFiles_EdgeCases(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	token, orgID := createTestUserAndOrg(t, h)

	tmpDir := t.TempDir()
	storage, err := plugin.NewLocalStorageService(tmpDir, []byte("test-secret"), "")
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	h.SetStorage(storage)

	// 1. Precheck with existing hash and empty name/content-type
	f1 := models.File{
		OrgID:       orgID,
		Name:        "default-name.txt",
		ContentType: "text/plain",
		Size:        123,
		MD5:         "11112222333344445555666677778888",
		Path:        "dedup/11/default.txt",
	}
	if err := h.db.Create(&f1).Error; err != nil {
		t.Fatalf("create f1: %v", err)
	}

	precheckReq := `{"md5": "11112222333344445555666677778888"}`
	req := httptest.NewRequest("POST", "/api/files/upload", strings.NewReader(precheckReq))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var precheckResp UploadFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &precheckResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if precheckResp.File.Name != "default-name.txt" || precheckResp.File.ContentType != "text/plain" {
		t.Errorf("expected inherited name/ct, got %s / %s", precheckResp.File.Name, precheckResp.File.ContentType)
	}

	// 2. Invalid JSON body
	req = httptest.NewRequest("POST", "/api/files/upload", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d", rec.Code)
	}

	// 3. Multipart missing file and no MD5
	var bodyEmpty bytes.Buffer
	writerEmpty := multipart.NewWriter(&bodyEmpty)
	_ = writerEmpty.WriteField("name", "nofile.txt")
	_ = writerEmpty.Close()
	req = httptest.NewRequest("POST", "/api/files/upload", &bodyEmpty)
	req.Header.Set("Content-Type", writerEmpty.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for multipart missing file, got %d", rec.Code)
	}

	// 4. Multipart missing file but has unknown MD5 -> 404
	var bodyUnknownMD5 bytes.Buffer
	writerUnknown := multipart.NewWriter(&bodyUnknownMD5)
	_ = writerUnknown.WriteField("md5", "99998888777766665555444433332222")
	_ = writerUnknown.Close()
	req = httptest.NewRequest("POST", "/api/files/upload", &bodyUnknownMD5)
	req.Header.Set("Content-Type", writerUnknown.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for multipart unknown MD5 without file, got %d", rec.Code)
	}

	// 5. Download: object not found in storage (DB record exists but storage missing)
	fMissing := models.File{
		OrgID:       orgID,
		Name:        "missing-on-disk.bin",
		ContentType: "",
		Size:        10,
		MD5:         "abcdabcdabcdabcdabcdabcdabcdabcd",
		Path:        "nonexistent/path/on/disk.bin",
	}
	if err := h.db.Create(&fMissing).Error; err != nil {
		t.Fatalf("create fMissing: %v", err)
	}
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d/download", fMissing.ID), nil)
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing storage object, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. TempToken unauthenticated -> 401
	req = httptest.NewRequest("POST", "/api/files/temp-token", strings.NewReader(`{"file_id": 1}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// 7. GetFile unauthenticated -> 401
	req = httptest.NewRequest("GET", "/api/files/1", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// 8. Form hint dedup without name field (falls back to existing.Name)
	var bodyHintNoName bytes.Buffer
	writerHintNoName := multipart.NewWriter(&bodyHintNoName)
	_ = writerHintNoName.WriteField("md5", f1.MD5)
	_ = writerHintNoName.Close()
	req = httptest.NewRequest("POST", "/api/files/upload", &bodyHintNoName)
	req.Header.Set("Content-Type", writerHintNoName.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// 9. Raw stream upload with empty X-File-Name and Content-Type
	req = httptest.NewRequest("POST", "/api/files/upload", strings.NewReader("raw stream test"))
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for raw stream without headers, got %d", rec.Code)
	}

	// 10. Multipart upload without name field (falls back to header filename)
	var bodyEmptyName bytes.Buffer
	writerEmptyName := multipart.NewWriter(&bodyEmptyName)
	partEmpty, err := writerEmptyName.CreateFormFile("file", "blob.bin")
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	_, _ = partEmpty.Write([]byte("unnamed content"))
	_ = writerEmptyName.Close()
	req = httptest.NewRequest("POST", "/api/files/upload", &bodyEmptyName)
	req.Header.Set("Content-Type", writerEmptyName.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for blob.bin multipart, got %d: %s", rec.Code, rec.Body.String())
	}

	// 11. Storage nil branches
	h.SetStorage(nil)
	// upload with storage nil -> 500
	req = httptest.NewRequest("POST", "/api/files/upload", strings.NewReader(`{"md5": "11112222333344445555666677778888"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when storage nil, got %d", rec.Code)
	}

	// temp-token with storage nil -> 500
	req = httptest.NewRequest("POST", "/api/files/temp-token", strings.NewReader(fmt.Sprintf(`{"file_id": %d}`, f1.ID)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when storage nil, got %d", rec.Code)
	}

	// download with storage nil -> 500
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/files/%d/download", f1.ID), nil)
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when storage nil, got %d", rec.Code)
	}
}

func TestFiles_DirectSafeguards(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)

	if _, err := h.uploadFile(context.Background(), &UploadFileInput{}); err == nil {
		t.Error("expected error for nil ctx in uploadFile")
	}
	if _, err := h.downloadFile(context.Background(), &DownloadFileInput{}); err == nil {
		t.Error("expected error for nil ctx in downloadFile")
	}
	if _, err := h.createTempToken(context.Background(), &TempTokenInput{}); err == nil {
		t.Error("expected error for nil ctx in createTempToken")
	}
	if _, err := h.getFile(context.Background(), &GetFileInput{}); err == nil {
		t.Error("expected error for nil ctx in getFile")
	}

	// Unauthenticated context direct calls
	req := httptest.NewRequest("GET", "/api/files/1", nil)
	rec := httptest.NewRecorder()
	ctx := humago.NewContext(&huma.Operation{}, req, rec)

	if _, err := h.getFile(context.Background(), &GetFileInput{Ctx: ctx, ID: 1}); err == nil {
		t.Error("expected 401 unauthenticated error for getFile")
	}
	if _, err := h.createTempToken(context.Background(), &TempTokenInput{Ctx: ctx, Body: TempTokenRequest{FileID: 1}}); err == nil {
		t.Error("expected 401 unauthenticated error for createTempToken")
	}
	if _, err := h.uploadFile(context.Background(), &UploadFileInput{Ctx: ctx}); err == nil {
		t.Error("expected 401 unauthenticated error for uploadFile")
	}
}
