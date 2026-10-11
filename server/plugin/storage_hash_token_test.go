package plugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
