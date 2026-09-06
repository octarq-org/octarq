package plugin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

var _ StorageService = (*LocalStorageService)(nil)

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

// GetStorageService resolves the registered StorageService from the context lookup.
func GetStorageService(ctx *Context) (StorageService, bool) {
	if ctx == nil {
		return nil, false
	}
	return LookupAs[StorageService](ctx, ServiceStorage)
}
