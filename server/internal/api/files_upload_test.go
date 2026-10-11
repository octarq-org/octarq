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
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func createTestUserAndOrg(t *testing.T, h *Handler) (string, uint) {
	t.Helper()
	email := fmt.Sprintf("user-%d@example.com", time.Now().UnixNano())
	user := models.User{
		Email:        email,
		PasswordHash: "pw",
	}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := models.Org{
		Name: "Test Org",
		Slug: fmt.Sprintf("org-%d", time.Now().UnixNano()),
	}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	member := models.OrgMember{
		OrgID:  org.ID,
		UserID: user.ID,
		Role:   "owner",
	}
	if err := h.db.Create(&member).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}

	sessionToken := fmt.Sprintf("test-token-%d", time.Now().UnixNano())
	sess := models.Session{
		UserID:    user.ID,
		OrgID:     org.ID,
		Token:     models.HashToken(sessionToken),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := h.db.Create(&sess).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	return sessionToken, org.ID
}

func TestFiles_Upload_Precheck(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	token, orgID := createTestUserAndOrg(t, h)

	// Ensure isolated test storage
	tmpDir := t.TempDir()
	storage, err := plugin.NewLocalStorageService(tmpDir, []byte("test-secret"), "")
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	h.SetStorage(storage)

	// 1. Missing MD5 in JSON
	bodyJSON := `{"name": "test.txt"}`
	req := httptest.NewRequest("POST", "/api/files/upload", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty MD5, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Invalid MD5 length
	bodyJSON = `{"md5": "short"}`
	req = httptest.NewRequest("POST", "/api/files/upload", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short MD5, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Unknown MD5 -> 404
	unknownMD5 := "00000000000000000000000000000000"
	bodyJSON = fmt.Sprintf(`{"md5": "%s", "name": "unknown.png"}`, unknownMD5)
	req = httptest.NewRequest("POST", "/api/files/upload", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown MD5, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Precheck hit via existing DB record in same org
	knownMD5 := "098f6bcd4621d373cade4e832627b4f6"
	existingFile := models.File{
		OrgID:       orgID,
		Name:        "existing.png",
		Size:        512,
		ContentType: "image/png",
		MD5:         knownMD5,
		Path:        "dedup/09/098f6bcd4621d373cade4e832627b4f6.png",
	}
	if err := h.db.Create(&existingFile).Error; err != nil {
		t.Fatalf("create existing file: %v", err)
	}

	bodyJSON = fmt.Sprintf(`{"md5": "%s", "name": "new-alias.png"}`, knownMD5)
	req = httptest.NewRequest("POST", "/api/files/upload", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for precheck hit, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp UploadFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.Deduplicated {
		t.Error("expected Deduplicated=true for precheck hit")
	}
	if resp.File.MD5 != knownMD5 {
		t.Errorf("expected MD5 %s, got %s", knownMD5, resp.File.MD5)
	}
	if resp.File.Path != existingFile.Path {
		t.Errorf("expected reused path %s, got %s", existingFile.Path, resp.File.Path)
	}

	// 5. Cross-tenant isolation (SEC-02): file exists in another org, precheck must fail closed with 404
	_, otherOrgID := createTestUserAndOrg(t, h)
	crossOrgMD5 := "5eb63bbbe01eeed093cb22bb8f5acdc3"
	otherFile := models.File{
		OrgID:       otherOrgID,
		Name:        "secret.txt",
		Size:        11,
		ContentType: "text/plain",
		MD5:         crossOrgMD5,
		Path:        "dedup/other-secret.txt",
	}
	if err := h.db.Create(&otherFile).Error; err != nil {
		t.Fatalf("create other org file: %v", err)
	}
	// Also ensure file is present on disk
	if _, _, _, _, err := storage.PutObjectByHash(context.Background(), strings.NewReader("hello world"), ".txt"); err != nil {
		t.Fatalf("put object on disk: %v", err)
	}

	// Requesting precheck for another org's MD5 must NOT return 200/deduplicate
	bodyJSON = fmt.Sprintf(`{"md5": "%s", "name": "stolen.txt"}`, crossOrgMD5)
	req = httptest.NewRequest("POST", "/api/files/upload", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-tenant precheck, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFiles_Upload_Multipart(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	token, _ := createTestUserAndOrg(t, h)

	tmpDir := t.TempDir()
	storage, err := plugin.NewLocalStorageService(tmpDir, []byte("test-secret"), "")
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	h.SetStorage(storage)

	// 1. Unauthenticated upload -> 401
	req := httptest.NewRequest("POST", "/api/files/upload", strings.NewReader("dummy"))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated upload, got %d", rec.Code)
	}

	// 2. Upload file content
	content := []byte("unique-file-payload-for-upload-test-12345")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "document.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write(content)
	_ = writer.WriteField("name", "my-document.txt")
	_ = writer.Close()

	req = httptest.NewRequest("POST", "/api/files/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for upload, got %d: %s", rec.Code, rec.Body.String())
	}

	var uploadResp UploadFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("unmarshal upload resp: %v", err)
	}
	if uploadResp.Deduplicated {
		t.Error("first upload should not be deduplicated")
	}
	if uploadResp.File.ID == 0 {
		t.Error("expected non-zero File ID")
	}
	if uploadResp.File.Name != "my-document.txt" {
		t.Errorf("expected name my-document.txt, got %s", uploadResp.File.Name)
	}
	firstMD5 := uploadResp.File.MD5
	firstPath := uploadResp.File.Path

	// 3. Upload same content again -> Dedup
	var body2 bytes.Buffer
	writer2 := multipart.NewWriter(&body2)
	part2, err := writer2.CreateFormFile("file", "duplicate.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part2.Write(content)
	_ = writer2.Close()

	req = httptest.NewRequest("POST", "/api/files/upload", &body2)
	req.Header.Set("Content-Type", writer2.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for dedup upload, got %d: %s", rec.Code, rec.Body.String())
	}

	var dedupResp UploadFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &dedupResp); err != nil {
		t.Fatalf("unmarshal dedup resp: %v", err)
	}
	if !dedupResp.Deduplicated {
		t.Error("expected second upload to be deduplicated")
	}
	if dedupResp.File.MD5 != firstMD5 {
		t.Errorf("expected matching MD5 %s, got %s", firstMD5, dedupResp.File.MD5)
	}
	if dedupResp.File.Path != firstPath {
		t.Errorf("expected matching Path %s, got %s", firstPath, dedupResp.File.Path)
	}

	// 4. Upload with md5 form hint
	var bodyHint bytes.Buffer
	writerHint := multipart.NewWriter(&bodyHint)
	_ = writerHint.WriteField("md5", firstMD5)
	_ = writerHint.WriteField("name", "hint-alias.txt")
	_ = writerHint.Close()

	req = httptest.NewRequest("POST", "/api/files/upload", &bodyHint)
	req.Header.Set("Content-Type", writerHint.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for md5 form hint hit, got %d: %s", rec.Code, rec.Body.String())
	}
	var hintResp UploadFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &hintResp); err != nil {
		t.Fatalf("unmarshal hint resp: %v", err)
	}
	if !hintResp.Deduplicated {
		t.Error("expected hint upload to be deduplicated")
	}
}

func TestFiles_Upload_RawStream(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	token, _ := createTestUserAndOrg(t, h)

	tmpDir := t.TempDir()
	storage, _ := plugin.NewLocalStorageService(tmpDir, []byte("test-secret"), "")
	h.SetStorage(storage)

	rawContent := []byte("binary-stream-raw-data")
	req := httptest.NewRequest("POST", "/api/files/upload", bytes.NewReader(rawContent))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-File-Name", "stream.bin")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for raw stream upload, got %d: %s", rec.Code, rec.Body.String())
	}

	var rawResp UploadFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("unmarshal raw resp: %v", err)
	}
	if rawResp.File.Name != "stream.bin" {
		t.Errorf("expected stream.bin, got %s", rawResp.File.Name)
	}
	if rawResp.File.Size != int64(len(rawContent)) {
		t.Errorf("expected size %d, got %d", len(rawContent), rawResp.File.Size)
	}
}
