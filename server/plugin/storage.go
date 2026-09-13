package plugin

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrStorageObjectNotFound indicates that the requested object key does not exist.
var ErrStorageObjectNotFound = errors.New("storage: object not found")

// StorageObjectInfo contains metadata about a stored blob or object.
type StorageObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
	ContentType  string    `json:"contentType,omitempty"`
}

// StorageService is the core SPI abstraction for object storage, presigned URLs,
// and blob lifecycle across Core plugins and Pro commercial modules.
type StorageService interface {
	PresignPutURL(ctx context.Context, key string, expire time.Duration) (string, error)
	PresignGetURL(ctx context.Context, key string, expire time.Duration) (string, error)
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, key string) error
	StatObject(ctx context.Context, key string) (*StorageObjectInfo, error)
}

// DownloadTokenSigner defines the contract for generating and verifying temporary download tokens.
type DownloadTokenSigner interface {
	GenerateDownloadToken(fileID string, expire time.Duration) string
	VerifyDownloadToken(token string, expectedFileID string) bool
	ParseDownloadToken(token string) (fileID string, expTime int64, valid bool)
}

var _ StorageService = (*LocalStorageService)(nil)
var _ DownloadTokenSigner = (*LocalStorageService)(nil)

// LocalStorageService is a Pure Go local filesystem fallback driver implementing StorageService.
// It uses atomic file writes, strict path traversal guards, and HMAC-SHA256 URL signing.
type LocalStorageService struct {
	baseDir   string
	secretKey []byte
	baseURL   string
}

// NewLocalStorageService initializes a local filesystem storage driver under baseDir.
// If secretKey is empty, a deterministic internal default key is used.
func NewLocalStorageService(baseDir string, secretKey []byte, baseURL string) (*LocalStorageService, error) {
	cleanDir := filepath.Clean(strings.TrimSpace(baseDir))
	if cleanDir == "" || cleanDir == "." {
		return nil, errors.New("storage: baseDir cannot be empty")
	}
	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return nil, fmt.Errorf("storage: create baseDir: %w", err)
	}
	if len(secretKey) == 0 {
		secretKey = []byte("octarq-local-storage-default-key")
	}
	return &LocalStorageService{
		baseDir:   cleanDir,
		secretKey: secretKey,
		baseURL:   strings.TrimRight(baseURL, "/"),
	}, nil
}

func (s *LocalStorageService) resolvePath(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", errors.New("storage: invalid empty key")
	}
	cleanKey := filepath.Clean(filepath.ToSlash(trimmed))
	if cleanKey == "." || cleanKey == "/" || cleanKey == "\\" {
		return "", errors.New("storage: invalid empty key")
	}
	if filepath.IsAbs(cleanKey) || strings.HasPrefix(cleanKey, "/") || strings.HasPrefix(cleanKey, "\\") || strings.Contains(cleanKey, ":") {
		return "", errors.New("storage: absolute paths or drive letters forbidden")
	}
	if strings.HasPrefix(cleanKey, "..") || strings.Contains(cleanKey, "/../") || strings.Contains(cleanKey, "\\..\\") {
		return "", errors.New("storage: path traversal forbidden")
	}
	fullPath := filepath.Join(s.baseDir, cleanKey)
	rel, err := filepath.Rel(s.baseDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("storage: path escapes base directory")
	}
	return fullPath, nil
}

// PutObject writes an object stream to the local filesystem atomically via a temporary file.
func (s *LocalStorageService) PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if r == nil {
		return errors.New("storage: nil reader")
	}
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("storage: create directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return fmt.Errorf("storage: create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmpFile.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if size > 0 {
		_, err = io.CopyN(tmpFile, r, size)
	} else {
		_, err = io.Copy(tmpFile, r)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("storage: write object: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("storage: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, fullPath); err != nil {
		return fmt.Errorf("storage: commit object: %w", err)
	}
	cleanup = false
	return nil
}

// GetObject opens a stored object for reading.
func (s *LocalStorageService) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrStorageObjectNotFound
		}
		return nil, fmt.Errorf("storage: open object: %w", err)
	}
	return f, nil
}

// DeleteObject deletes an object by key. It is idempotent (succeeds if file does not exist).
func (s *LocalStorageService) DeleteObject(ctx context.Context, key string) error {
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	err = os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete object: %w", err)
	}
	return nil
}

// StatObject inspects the metadata of a stored object.
func (s *LocalStorageService) StatObject(ctx context.Context, key string) (*StorageObjectInfo, error) {
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrStorageObjectNotFound
		}
		return nil, fmt.Errorf("storage: stat object: %w", err)
	}
	return &StorageObjectInfo{
		Key:          key,
		Size:         fi.Size(),
		LastModified: fi.ModTime(),
	}, nil
}

// PresignPutURL generates an HMAC-signed upload URL for direct client upload.
func (s *LocalStorageService) PresignPutURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	return s.presign("put", key, expire)
}

// PresignGetURL generates an HMAC-signed download URL for direct client download.
func (s *LocalStorageService) PresignGetURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	return s.presign("get", key, expire)
}

func (s *LocalStorageService) presign(action, key string, expire time.Duration) (string, error) {
	if _, err := s.resolvePath(key); err != nil {
		return "", err
	}
	if expire <= 0 {
		expire = 15 * time.Minute
	}
	expTime := time.Now().Add(expire).Unix()
	sig := s.sign(action, key, expTime)

	q := url.Values{}
	q.Set("action", action)
	q.Set("key", key)
	q.Set("expires", strconv.FormatInt(expTime, 10))
	q.Set("sig", sig)

	relPath := "/api/storage/local?" + q.Encode()
	if s.baseURL != "" {
		return s.baseURL + relPath, nil
	}
	return relPath, nil
}

func (s *LocalStorageService) sign(action, key string, expTime int64) string {
	mac := hmac.New(sha256.New, s.secretKey)
	_, _ = fmt.Fprintf(mac, "%s:%s:%d", action, key, expTime)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyPresignedURL verifies an incoming signed action, key, expiration, and signature.
func (s *LocalStorageService) VerifyPresignedURL(action, key string, expTime int64, sig string) bool {
	if time.Now().Unix() > expTime {
		return false
	}
	expected := s.sign(action, key, expTime)
	return hmac.Equal([]byte(expected), []byte(sig))
}

// GenerateDownloadToken generates an HMAC-SHA256 signed temporary download token.
// The token encodes file_id + expiry + signature.
// Default expiration is 15 minutes if expire <= 0.
func (s *LocalStorageService) GenerateDownloadToken(fileID string, expire time.Duration) string {
	if expire <= 0 {
		expire = 15 * time.Minute
	}
	expTime := time.Now().Add(expire).Unix()
	sig := s.signDownloadToken(fileID, expTime)
	raw := fmt.Sprintf("%s.%d.%s", fileID, expTime, sig)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func (s *LocalStorageService) signDownloadToken(fileID string, expTime int64) string {
	mac := hmac.New(sha256.New, s.secretKey)
	_, _ = fmt.Fprintf(mac, "download:%s:%d", fileID, expTime)
	return hex.EncodeToString(mac.Sum(nil))
}

// ParseDownloadToken parses and validates an HMAC-SHA256 temporary download token.
// It verifies both the expiration timestamp and HMAC signature.
func (s *LocalStorageService) ParseDownloadToken(token string) (fileID string, expTime int64, valid bool) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", 0, false
	}
	raw := trimmed
	if decoded, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil && strings.Count(string(decoded), ".") == 2 {
		raw = string(decoded)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", 0, false
	}
	fileID = parts[0]
	expStr := parts[1]
	sig := parts[2]
	if fileID == "" || expStr == "" || sig == "" {
		return "", 0, false
	}
	expTime, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", 0, false
	}
	if time.Now().Unix() > expTime {
		return fileID, expTime, false
	}
	expectedSig := s.signDownloadToken(fileID, expTime)
	if !hmac.Equal([]byte(expectedSig), []byte(sig)) {
		return fileID, expTime, false
	}
	return fileID, expTime, true
}

// VerifyDownloadToken checks whether the token is valid, unexpired, and matches expectedFileID.
func (s *LocalStorageService) VerifyDownloadToken(token string, expectedFileID string) bool {
	fileID, _, valid := s.ParseDownloadToken(token)
	return valid && fileID == expectedFileID
}

// ComputeMD5 calculates the MD5 checksum of r as a 32-character lowercase hex string.
func (s *LocalStorageService) ComputeMD5(r io.Reader) (string, error) {
	if r == nil {
		return "", errors.New("storage: nil reader")
	}
	h := md5.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("storage: compute md5: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PutObjectByHash writes the content of r into storage addressed by its MD5 checksum.
// If an object with the same MD5 already exists, it skips writing, removes the temporary file,
// and returns deduplicated=true with the existing relative key and size.
func (s *LocalStorageService) PutObjectByHash(ctx context.Context, r io.Reader, ext string) (key string, md5Hash string, sizeWritten int64, deduplicated bool, err error) {
	if r == nil {
		return "", "", 0, false, errors.New("storage: nil reader")
	}
	cleanExt := filepath.Clean(ext)
	if cleanExt == "." || cleanExt == "/" || cleanExt == "\\" {
		cleanExt = ""
	}
	if cleanExt != "" && !strings.HasPrefix(cleanExt, ".") {
		cleanExt = "." + cleanExt
	}

	dedupDir := filepath.Join(s.baseDir, "dedup")
	if err := os.MkdirAll(dedupDir, 0755); err != nil {
		return "", "", 0, false, fmt.Errorf("storage: create dedup dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dedupDir, ".upload-*")
	if err != nil {
		return "", "", 0, false, fmt.Errorf("storage: create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmpFile.Close()
			_ = os.Remove(tmpName)
		}
	}()

	hasher := md5.New()
	mw := io.MultiWriter(tmpFile, hasher)
	written, err := io.Copy(mw, r)
	if err != nil {
		return "", "", 0, false, fmt.Errorf("storage: write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", "", 0, false, fmt.Errorf("storage: close temp file: %w", err)
	}

	md5Hash = hex.EncodeToString(hasher.Sum(nil))
	prefix := md5Hash[:2]
	relKey := filepath.ToSlash(filepath.Join("dedup", prefix, md5Hash+cleanExt))
	fullTarget := filepath.Join(s.baseDir, relKey)

	// Check if target file already exists
	if fi, statErr := os.Stat(fullTarget); statErr == nil && !fi.IsDir() {
		cleanup = true
		return relKey, md5Hash, fi.Size(), true, nil
	}

	targetDir := filepath.Dir(fullTarget)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", "", 0, false, fmt.Errorf("storage: create target dir: %w", err)
	}

	if err := os.Rename(tmpName, fullTarget); err != nil {
		return "", "", 0, false, fmt.Errorf("storage: commit dedup file: %w", err)
	}
	cleanup = false
	return relKey, md5Hash, written, false, nil
}

// CheckMD5Exists checks whether an object with the given MD5 hash exists in dedup storage.
func (s *LocalStorageService) CheckMD5Exists(md5Hash string, ext string) (key string, size int64, exists bool) {
	cleanHash := strings.TrimSpace(strings.ToLower(md5Hash))
	if len(cleanHash) != 32 {
		return "", 0, false
	}
	cleanExt := filepath.Clean(ext)
	if cleanExt == "." || cleanExt == "/" || cleanExt == "\\" {
		cleanExt = ""
	}
	if cleanExt != "" && !strings.HasPrefix(cleanExt, ".") {
		cleanExt = "." + cleanExt
	}
	prefix := cleanHash[:2]
	relKey := filepath.ToSlash(filepath.Join("dedup", prefix, cleanHash+cleanExt))
	fullTarget := filepath.Join(s.baseDir, relKey)
	if fi, err := os.Stat(fullTarget); err == nil && !fi.IsDir() {
		return relKey, fi.Size(), true
	}
	return "", 0, false
}

// GetStorageService resolves the registered StorageService from the context lookup.
func GetStorageService(ctx *Context) (StorageService, bool) {
	if ctx == nil {
		return nil, false
	}
	return LookupAs[StorageService](ctx, ServiceStorage)
}
