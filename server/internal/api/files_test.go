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

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
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
	token, _ := createTestUserAndOrg(t, h)

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

	// 4. Precheck hit via existing DB record
	knownMD5 := "098f6bcd4621d373cade4e832627b4f6"
	existingFile := models.File{
		OrgID:       1,
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

	// 5. Precheck hit via disk object (even if not yet in DB)
	diskMD5 := "5eb63bbbe01eeed093cb22bb8f5acdc3"
	key, _, _, _, err := storage.PutObjectByHash(context.Background(), strings.NewReader("hello world"), ".txt")
	if err != nil {
		t.Fatalf("put object on disk: %v", err)
	}
	bodyJSON = fmt.Sprintf(`{"md5": "%s", "name": "disk-hello.txt"}`, diskMD5)
	req = httptest.NewRequest("POST", "/api/files/upload", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "octarq_session", Value: token})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for disk precheck hit, got %d: %s", rec.Code, rec.Body.String())
	}
	var diskResp UploadFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &diskResp); err != nil {
		t.Fatalf("unmarshal disk response: %v", err)
	}
	if !diskResp.Deduplicated {
		t.Error("expected Deduplicated=true for disk precheck hit")
	}
	if diskResp.File.Path != key {
		t.Errorf("expected path %s, got %s", key, diskResp.File.Path)
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
