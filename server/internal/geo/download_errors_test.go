package geo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newGeoHandler builds an httptest handler that serves the given tar.gz bytes
// with the matching sha256 checksum line, mimicking MaxMind's geoip_download
// endpoint.
func newGeoHandler(tarBytes []byte, shaHex string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("suffix") {
		case "tar.gz.sha256":
			fmt.Fprintf(w, "%s  GeoLite2-City.tar.gz\n", shaHex)
		case "tar.gz":
			w.Write(tarBytes)
		default:
			http.NotFound(w, r)
		}
	}
}

// makeTarGz builds a valid tar.gz stream whose first entry is a .mmdb file.
func makeTarGz(t *testing.T, mmdbName string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	body := []byte("dummy mmdb content")
	if err := tw.WriteHeader(&tar.Header{
		Name: "GeoLite2-City_20260101/" + mmdbName,
		Mode: 0600, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func hashOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func downloadFrom(t *testing.T, handler http.HandlerFunc) (string, error) {
	t.Helper()
	srv := httptest.NewServer(handler)
	defer srv.Close()
	return download(context.Background(), downloadOptions{
		licenseKey: "testkey",
		dir:        t.TempDir(),
		baseURL:    srv.URL,
	})
}

func TestDownloadChecksumMismatch(t *testing.T) {
	tarBytes := makeTarGz(t, "GeoLite2-City.mmdb")
	wrongSum := hashOf([]byte("different content"))
	path, err := downloadFrom(t, newGeoHandler(tarBytes, wrongSum))
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected sha256 mismatch error, got path=%q err=%v", path, err)
	}
}

func TestDownloadChecksumErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{"http error on checksum", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("suffix") == "tar.gz.sha256" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.NotFound(w, r)
		}, "HTTP 403"},
		{"empty checksum body", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "   ")
		}, "empty sha256 response"},
		{"short checksum", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "abc")
		}, "malformed sha256 response"},
		{"non-hex checksum", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, strings.Repeat("zz", 32))
		}, "malformed sha256 response"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := downloadFrom(t, c.handler)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("expected error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestDownloadArchiveErrors(t *testing.T) {
	// Non-200 on the tar.gz fetch (checksum fetch must succeed first).
	_, err := downloadFrom(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("suffix") {
		case "tar.gz.sha256":
			fmt.Fprint(w, strings.Repeat("ab", 32))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("expected HTTP 404 error, got %v", err)
	}

	// Client-level failure (connection refused) surfaces as an error.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err = download(context.Background(), downloadOptions{
		licenseKey: "k",
		dir:        t.TempDir(),
		baseURL:    url,
	})
	if err == nil {
		t.Fatal("expected connection error from closed server")
		return
	}

	// Gzip garbage in the archive stream.
	tarBytes := []byte("this is not gzip")
	_, err = downloadFrom(t, newGeoHandler(tarBytes, hashOf(tarBytes)))
	if err == nil || !strings.Contains(err.Error(), "not a gzip archive") {
		t.Fatalf("expected gzip error, got %v", err)
	}

	// Valid gzip but no .mmdb entry in the tar.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	body := []byte("x")
	if err := tw.WriteHeader(&tar.Header{
		Name: "README.txt", Mode: 0600, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	tw.Write(body)
	tw.Close()
	gw.Close()
	noMMDB := buf.Bytes()
	_, err = downloadFrom(t, newGeoHandler(noMMDB, hashOf(noMMDB)))
	if err == nil || !strings.Contains(err.Error(), "no .mmdb file found") {
		t.Fatalf("expected no-mmdb error, got %v", err)
	}

	// Truncated tar (the mmdb entry header is cut mid-stream).
	truncated := makeTarGz(t, "GeoLite2-City.mmdb")
	truncated = truncated[:len(truncated)/2]
	_, err = downloadFrom(t, newGeoHandler(truncated, hashOf(truncated)))
	if err == nil {
		t.Fatal("expected error from truncated archive")
		return
	}
}

func TestRedactKeyHelpers(t *testing.T) {
	if err := redactKey(nil, "key"); err != nil {
		t.Errorf("redactKey(nil) = %v, want nil", err)
	}
	if err := redactKey(errors.New("boom"), ""); err == nil || err.Error() != "boom" {
		t.Errorf("redactKey with empty key must pass through, got %v", err)
	}
	// The URL-escaped key form is scrubbed too (net/http errors embed the URL
	// with QueryEscape applied), and keys with special characters use that form.
	key := "my key"
	escaped := url.QueryEscape(key)
	err := redactKey(fmt.Errorf("url %s and raw %s", escaped, key), key)
	msg := err.Error()
	if strings.Contains(msg, key) || strings.Contains(msg, escaped) {
		t.Errorf("key leaked into: %v", msg)
	}
}

func TestDataDirFromEnv(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "")
	t.Setenv("OCTARQ_DB_DSN", "")
	if got := dataDirFromEnv(); got != "." {
		t.Errorf("defaults dataDirFromEnv = %q, want \".\"", got)
	}

	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", "file:/var/lib/octarq/data/octarq.db")
	if got := dataDirFromEnv(); got != "/var/lib/octarq/data" {
		t.Errorf("sqlite dataDirFromEnv = %q, want %q", got, "/var/lib/octarq/data")
	}

	t.Setenv("OCTARQ_DB_DRIVER", "postgres")
	t.Setenv("OCTARQ_DB_DSN", "postgres://u@h/db")
	if got := dataDirFromEnv(); got != "." {
		t.Errorf("postgres dataDirFromEnv = %q, want \".\"", got)
	}
}

func TestDataDirForEdgeCases(t *testing.T) {
	if got := dataDirFor("sqlite", ""); got != "." {
		t.Errorf("dataDirFor(sqlite,\"\") = %q, want \".\"", got)
	}
	if got := dataDirFor("sqlite", "octarq.db"); got != "." {
		t.Errorf("dataDirFor(sqlite,\"octarq.db\") = %q, want \".\"", got)
	}
	if got := dataDirFor("sqlite", "file:octarq.db"); got != "." {
		t.Errorf("dataDirFor(sqlite,\"file:octarq.db\") = %q, want \".\"", got)
	}
	if got := dataDirFor("sqlite", "sub/dir/octarq.db"); got != "sub/dir" {
		t.Errorf("dataDirFor(sqlite,\"sub/dir/octarq.db\") = %q, want \"sub/dir\"", got)
	}
}

func TestDecideModeCachedFile(t *testing.T) {
	dir := t.TempDir()
	writeMMDB(t, dir, false)
	m, p := decideMode("", dir, "")
	if m != modeCached || p != filepath.Join(dir, cachedFileName) {
		t.Errorf("decideMode cached = (%q,%q), want (cached,%q)", m, p, filepath.Join(dir, cachedFileName))
	}
	// A manual path always wins over the cache.
	m, p = decideMode("/manual.mmdb", dir, "")
	if m != modeManual || p != "/manual.mmdb" {
		t.Errorf("decideMode manual = (%q,%q), want (manual,/manual.mmdb)", m, p)
	}
}

// TestDownloadLocalFailurePaths drives download into its filesystem failure
// branches (temp-file creation and the final rename), none of which touch the
// network. The responses are always cryptographically valid so the failure is
// guaranteed to be the file operation under test.
func TestDownloadLocalFailurePaths(t *testing.T) {
	tarBytes := makeTarGz(t, "GeoLite2-City.mmdb")
	goodSum := hashOf(tarBytes)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("suffix") {
		case "tar.gz.sha256":
			fmt.Fprintf(w, "%s  GeoLite2-City.tar.gz\n", goodSum)
		case "tar.gz":
			w.Write(tarBytes)
		}
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// CreateTemp fails when dir is actually a file.
	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := download(context.Background(), downloadOptions{licenseKey: "k", dir: filePath, baseURL: srv.URL}); err == nil {
		t.Error("expected error when download dir is a file")
	}

	// Rename fails when the destination name is taken by a directory.
	blocked := t.TempDir()
	if err := os.Mkdir(filepath.Join(blocked, cachedFileName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := download(context.Background(), downloadOptions{licenseKey: "k", dir: blocked, baseURL: srv.URL}); err == nil {
		t.Error("expected error when destination is an existing directory")
	}
}

// TestFetchRejectsMalformedURL covers the request-construction error path in
// fetch, which requires a base URL that url.Parse rejects.
func TestFetchRejectsMalformedURL(t *testing.T) {
	client := &http.Client{}
	_, err := fetch(context.Background(), client, "http://bad host name", "key", "tar.gz")
	if err == nil {
		t.Fatal("expected url-construction error from malformed base URL")
		return
	}
}
