package plugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewLocalStorageService(t *testing.T) {
	tmp := t.TempDir()

	// Valid creation
	svc, err := NewLocalStorageService(tmp, nil, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.baseURL != "https://example.com" {
		t.Errorf("expected baseURL https://example.com, got %s", svc.baseURL)
	}

	// Empty baseDir
	_, err = NewLocalStorageService("", nil, "")
	if err == nil {
		t.Fatal("expected error for empty baseDir")
	}

	// BaseDir creation failure (when baseDir points to a file, not a directory)
	fileAsDir := filepath.Join(tmp, "existing-file")
	if err := os.WriteFile(fileAsDir, []byte("not-a-dir"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err = NewLocalStorageService(fileAsDir, nil, "")
	if err == nil {
		t.Fatal("expected error when baseDir is an existing file")
	}
}

func TestLocalStorageService_PutGetDeleteStat(t *testing.T) {
	tmp := t.TempDir()
	svc, err := NewLocalStorageService(tmp, []byte("test-secret"), "")
	if err != nil {
		t.Fatalf("create svc: %v", err)
	}

	ctx := context.Background()
	key := "avatars/user-123.png"
	content := []byte("fake-image-binary-data")

	// Stat before put -> not found
	_, err = svc.StatObject(ctx, key)
	if err != ErrStorageObjectNotFound {
		t.Fatalf("expected ErrStorageObjectNotFound, got %v", err)
	}

	// Get before put -> not found
	_, err = svc.GetObject(ctx, key)
	if err != ErrStorageObjectNotFound {
		t.Fatalf("expected ErrStorageObjectNotFound, got %v", err)
	}

	// PutObject with size <= 0 (unbounded copy)
	if err := svc.PutObject(ctx, key, bytes.NewReader(content), 0, "image/png"); err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}

	// StatObject
	info, err := svc.StatObject(ctx, key)
	if err != nil {
		t.Fatalf("StatObject failed: %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), info.Size)
	}
	if info.Key != key {
		t.Errorf("expected key %s, got %s", key, info.Key)
	}
	if info.LastModified.IsZero() {
		t.Error("expected non-zero last modified")
	}

	// GetObject
	rc, err := svc.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject failed: %v", err)
	}
	defer rc.Close()

	readBack, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(readBack) != string(content) {
		t.Fatalf("content mismatch: got %s, want %s", readBack, content)
	}

	// PutObject with explicit size > 0
	content2 := []byte("updated-image-binary-data-extra-bytes")
	if err := svc.PutObject(ctx, key, bytes.NewReader(content2), int64(len(content2)), "image/png"); err != nil {
		t.Fatalf("PutObject with size failed: %v", err)
	}
	info2, err := svc.StatObject(ctx, key)
	if err != nil || info2.Size != int64(len(content2)) {
		t.Fatalf("expected updated size %d, got %v (err=%v)", len(content2), info2, err)
	}

	// Nil reader error
	if err := svc.PutObject(ctx, key, nil, 0, ""); err == nil {
		t.Fatal("expected error for nil reader")
	}

	// Read error during PutObject
	errReader := &failingReader{err: errors.New("read fault")}
	if err := svc.PutObject(ctx, "fault.txt", errReader, 0, ""); err == nil {
		t.Fatal("expected error for failing reader")
	}

	// MkdirAll failure when key directory collides with existing file
	if err := os.WriteFile(filepath.Join(tmp, "blocker"), []byte("file"), 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if err := svc.PutObject(ctx, "blocker/child.txt", strings.NewReader("data"), 0, ""); err == nil {
		t.Fatal("expected error when directory collides with file")
	}

	// DeleteObject
	if err := svc.DeleteObject(ctx, key); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}

	// Stat after delete -> not found
	if _, err := svc.StatObject(ctx, key); err != ErrStorageObjectNotFound {
		t.Fatalf("expected ErrStorageObjectNotFound after delete, got %v", err)
	}

	// Idempotent delete (deleting non-existent file should succeed)
	if err := svc.DeleteObject(ctx, key); err != nil {
		t.Fatalf("idempotent DeleteObject failed: %v", err)
	}
}

func TestLocalStorageService_PathTraversal(t *testing.T) {
	tmp := t.TempDir()
	svc, err := NewLocalStorageService(tmp, nil, "")
	if err != nil {
		t.Fatalf("create svc: %v", err)
	}
	ctx := context.Background()

	traversalKeys := []string{
		"",
		" ",
		".",
		"/",
		"\\",
		"../evil.txt",
		"/etc/passwd",
		"c:/windows/win.ini",
		"a/../../b.txt",
		"foo/bar/../../../escaped.txt",
	}

	for _, k := range traversalKeys {
		if err := svc.PutObject(ctx, k, strings.NewReader("bad"), 0, ""); err == nil {
			t.Errorf("expected PutObject error for dangerous key %q, got nil", k)
		}
		if _, err := svc.GetObject(ctx, k); err == nil {
			t.Errorf("expected GetObject error for dangerous key %q, got nil", k)
		}
		if err := svc.DeleteObject(ctx, k); err == nil {
			t.Errorf("expected DeleteObject error for dangerous key %q, got nil", k)
		}
		if _, err := svc.StatObject(ctx, k); err == nil {
			t.Errorf("expected StatObject error for dangerous key %q, got nil", k)
		}
		if _, err := svc.PresignPutURL(ctx, k, time.Minute); err == nil {
			t.Errorf("expected PresignPutURL error for dangerous key %q, got nil", k)
		}
		if _, err := svc.PresignGetURL(ctx, k, time.Minute); err == nil {
			t.Errorf("expected PresignGetURL error for dangerous key %q, got nil", k)
		}
	}
}

func TestLocalStorageService_PresignedURLs(t *testing.T) {
	tmp := t.TempDir()
	secret := []byte("signing-secret-key-12345")
	svc, err := NewLocalStorageService(tmp, secret, "https://cdn.example.com")
	if err != nil {
		t.Fatalf("create svc: %v", err)
	}
	ctx := context.Background()

	key := "docs/contract.pdf"

	// PresignPutURL with default expire (<= 0)
	putURL, err := svc.PresignPutURL(ctx, key, 0)
	if err != nil {
		t.Fatalf("PresignPutURL failed: %v", err)
	}
	if !strings.HasPrefix(putURL, "https://cdn.example.com/api/storage/local?") {
		t.Fatalf("unexpected URL prefix: %s", putURL)
	}

	parsed, err := url.Parse(putURL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	q := parsed.Query()
	if q.Get("action") != "put" || q.Get("key") != key {
		t.Fatalf("query mismatch: %v", q)
	}
	exp, _ := strconv.ParseInt(q.Get("expires"), 10, 64)
	sig := q.Get("sig")

	// Verify valid
	if !svc.VerifyPresignedURL("put", key, exp, sig) {
		t.Fatal("VerifyPresignedURL failed for valid put URL")
	}

	// Verify invalid action
	if svc.VerifyPresignedURL("get", key, exp, sig) {
		t.Fatal("VerifyPresignedURL should fail for wrong action")
	}

	// Verify invalid key
	if svc.VerifyPresignedURL("put", "other-key", exp, sig) {
		t.Fatal("VerifyPresignedURL should fail for wrong key")
	}

	// Verify tampered sig
	if svc.VerifyPresignedURL("put", key, exp, sig+"bad") {
		t.Fatal("VerifyPresignedURL should fail for tampered sig")
	}

	// Verify expired
	pastExp := time.Now().Add(-1 * time.Minute).Unix()
	pastSig := svc.sign("put", key, pastExp)
	if svc.VerifyPresignedURL("put", key, pastExp, pastSig) {
		t.Fatal("VerifyPresignedURL should fail for expired time")
	}

	// PresignGetURL with empty baseURL
	svcNoBase, err := NewLocalStorageService(tmp, secret, "")
	if err != nil {
		t.Fatalf("create svc: %v", err)
	}
	getURL, err := svcNoBase.PresignGetURL(ctx, key, 30*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetURL failed: %v", err)
	}
	if !strings.HasPrefix(getURL, "/api/storage/local?") {
		t.Fatalf("expected relative URL when baseURL empty, got %s", getURL)
	}
}

func TestGetStorageService_ContextLookup(t *testing.T) {
	// Nil context
	if _, ok := GetStorageService(nil); ok {
		t.Fatal("expected false for nil context")
	}

	// Empty registry
	reg := NewRegistry()
	ctx := &Context{
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	}
	if _, ok := GetStorageService(ctx); ok {
		t.Fatal("expected false when service not registered")
	}

	// Registered service
	tmp := t.TempDir()
	svc, _ := NewLocalStorageService(tmp, nil, "")
	ctx.Provide(ServiceStorage, StorageService(svc))

	resolved, ok := GetStorageService(ctx)
	if !ok || resolved == nil {
		t.Fatal("failed to resolve registered StorageService")
	}
}

type failingReader struct {
	err error
}

func (r *failingReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

func TestLocalStorageService_ComputeMD5(t *testing.T) {
	tmp := t.TempDir()
	svc, err := NewLocalStorageService(tmp, []byte("test-secret"), "")
	if err != nil {
		t.Fatalf("create svc: %v", err)
	}

	// Nil reader
	if _, err := svc.ComputeMD5(nil); err == nil {
		t.Fatal("expected error for nil reader")
	}

	// Failing reader
	if _, err := svc.ComputeMD5(&failingReader{err: errors.New("read failed")}); err == nil {
		t.Fatal("expected error for failing reader")
	}

	// Known MD5 of "hello world" -> 5eb63bbbe01eeed093cb22bb8f5acdc3
	h, err := svc.ComputeMD5(strings.NewReader("hello world"))
	if err != nil {
		t.Fatalf("ComputeMD5 failed: %v", err)
	}
	expected := "5eb63bbbe01eeed093cb22bb8f5acdc3"
	if h != expected {
		t.Errorf("expected %s, got %s", expected, h)
	}
}

func TestLocalStorageService_PutObjectByHash_Dedup(t *testing.T) {
	tmp := t.TempDir()
	svc, err := NewLocalStorageService(tmp, []byte("test-secret"), "")
	if err != nil {
		t.Fatalf("create svc: %v", err)
	}
	ctx := context.Background()

	// Nil reader
	if _, _, _, _, err := svc.PutObjectByHash(ctx, nil, ".png"); err == nil {
		t.Fatal("expected error for nil reader")
	}

	// Failing reader
	if _, _, _, _, err := svc.PutObjectByHash(ctx, &failingReader{err: errors.New("disk err")}, ".png"); err == nil {
		t.Fatal("expected error for failing reader")
	}

	content := []byte("identical-file-content-for-dedup-testing")
	ext := ".png"

	// First upload: should not be deduplicated
	key1, hash1, size1, dedup1, err := svc.PutObjectByHash(ctx, bytes.NewReader(content), ext)
	if err != nil {
		t.Fatalf("first PutObjectByHash failed: %v", err)
	}
	if dedup1 {
		t.Error("first upload should not be marked as deduplicated")
	}
	if size1 != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size1)
	}
	if !strings.HasSuffix(key1, ".png") {
		t.Errorf("expected key to have .png suffix, got %s", key1)
	}

	// CheckMD5Exists
	foundKey, foundSize, exists := svc.CheckMD5Exists(hash1, ext)
	if !exists {
		t.Fatal("expected CheckMD5Exists to find file")
	}
	if foundKey != key1 || foundSize != size1 {
		t.Errorf("CheckMD5Exists mismatch: got key %s, size %d", foundKey, foundSize)
	}

	// CheckMD5Exists with invalid/short hash
	if _, _, exists := svc.CheckMD5Exists("short", ext); exists {
		t.Fatal("expected false for short hash")
	}
	if _, _, exists := svc.CheckMD5Exists("00000000000000000000000000000000", ext); exists {
		t.Fatal("expected false for nonexistent hash")
	}

	// Second upload with identical content: MUST be deduplicated
	key2, hash2, size2, dedup2, err := svc.PutObjectByHash(ctx, bytes.NewReader(content), ext)
	if err != nil {
		t.Fatalf("second PutObjectByHash failed: %v", err)
	}
	if !dedup2 {
		t.Error("second upload MUST be marked as deduplicated")
	}
	if key2 != key1 {
		t.Errorf("expected reused key %s, got %s", key1, key2)
	}
	if hash2 != hash1 {
		t.Errorf("expected matching hash %s, got %s", hash1, hash2)
	}
	if size2 != size1 {
		t.Errorf("expected matching size %d, got %d", size1, size2)
	}

	// Verify the stored object can be read back and matches content
	rc, err := svc.GetObject(ctx, key1)
	if err != nil {
		t.Fatalf("GetObject failed: %v", err)
	}
	defer rc.Close()
	readBack, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(readBack, content) {
		t.Fatal("read back content mismatch")
	}

	// Upload without extension
	key3, _, _, _, err := svc.PutObjectByHash(ctx, strings.NewReader("no-ext-content"), "")
	if err != nil {
		t.Fatalf("PutObjectByHash without ext failed: %v", err)
	}
	if strings.Contains(filepath.Base(key3), ".") {
		t.Errorf("expected key without ext, got %s", key3)
	}
}

func TestLocalStorageService_DownloadTokens(t *testing.T) {
	tmp := t.TempDir()
	secret := []byte("super-secret-key-12345")
	svc, err := NewLocalStorageService(tmp, secret, "")
	if err != nil {
		t.Fatalf("create svc: %v", err)
	}

	fileID := "42"

	// 1. Generate valid token with 15m expiration
	token := svc.GenerateDownloadToken(fileID, 15*time.Minute)
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Verify with correct fileID
	if !svc.VerifyDownloadToken(token, fileID) {
		t.Fatal("expected VerifyDownloadToken to succeed for matching fileID")
	}

	// Verify with wrong fileID -> must fail
	if svc.VerifyDownloadToken(token, "999") {
		t.Fatal("VerifyDownloadToken should fail for wrong fileID")
	}

	// Parse valid token
	parsedID, expTime, valid := svc.ParseDownloadToken(token)
	if !valid {
		t.Fatal("ParseDownloadToken reported invalid")
	}
	if parsedID != fileID {
		t.Errorf("expected fileID %s, got %s", fileID, parsedID)
	}
	if expTime <= time.Now().Unix() {
		t.Error("expected expiration in future")
	}

	// 2. Default expiration when expire <= 0
	defToken := svc.GenerateDownloadToken(fileID, 0)
	_, defExp, defValid := svc.ParseDownloadToken(defToken)
	if !defValid {
		t.Fatal("default token should be valid")
	}
	// default should be ~15 minutes ahead
	if defExp < time.Now().Add(14*time.Minute).Unix() || defExp > time.Now().Add(16*time.Minute).Unix() {
		t.Errorf("expected default expiry around 15m, got %d", defExp)
	}

	// 3. Expired token
	pastExp := time.Now().Add(-1 * time.Hour).Unix()
	pastSig := svc.signDownloadToken(fileID, pastExp)
	expiredRaw := fmt.Sprintf("%s.%d.%s", fileID, pastExp, pastSig)
	expiredToken := base64.RawURLEncoding.EncodeToString([]byte(expiredRaw))

	if svc.VerifyDownloadToken(expiredToken, fileID) {
		t.Fatal("VerifyDownloadToken should fail for expired token")
	}
	_, _, expValid := svc.ParseDownloadToken(expiredToken)
	if expValid {
		t.Fatal("ParseDownloadToken should report invalid for expired token")
	}

	// 4. Bad signature (tampered)
	tampered := token + "tampered"
	if svc.VerifyDownloadToken(tampered, fileID) {
		t.Fatal("VerifyDownloadToken should fail for tampered token")
	}

	// 5. Malformed tokens
	if svc.VerifyDownloadToken("", fileID) {
		t.Fatal("VerifyDownloadToken should fail for empty token")
	}
	if svc.VerifyDownloadToken("just.two.dots.and.too.many.parts", fileID) {
		t.Fatal("VerifyDownloadToken should fail for bad structure")
	}
	if svc.VerifyDownloadToken("not-base64-nor-dots", fileID) {
		t.Fatal("VerifyDownloadToken should fail for garbage token")
	}
	if svc.VerifyDownloadToken("..", fileID) {
		t.Fatal("VerifyDownloadToken should fail for empty parts")
	}
	if svc.VerifyDownloadToken("1.notanumber.sig", fileID) {
		t.Fatal("VerifyDownloadToken should fail for non-numeric expiry")
	}

	// 6. Dot-separated format (not base64url encoded) should also parse
	sig := svc.signDownloadToken(fileID, expTime)
	dotToken := fmt.Sprintf("%s.%d.%s", fileID, expTime, sig)
	if !svc.VerifyDownloadToken(dotToken, fileID) {
		t.Fatal("VerifyDownloadToken should accept dot-separated token")
	}

	// 7. Different secret key -> signature mismatch
	otherSvc, _ := NewLocalStorageService(tmp, []byte("different-secret"), "")
	if otherSvc.VerifyDownloadToken(token, fileID) {
		t.Fatal("VerifyDownloadToken should fail when secret key differs")
	}
}
