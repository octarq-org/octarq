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
	"path/filepath"
	"testing"
)

func TestDecideModeAndDataDir(t *testing.T) {
	t.Parallel()

	// decideMode
	m, p := decideMode("/path/to/db.mmdb", "/data", "")
	if m != modeManual || p != "/path/to/db.mmdb" {
		t.Errorf("manual mode failed: %v, %s", m, p)
	}

	m2, p2 := decideMode("", "/data", "key123")
	if m2 != modeDownload || p2 != filepath.Join("/data", "GeoLite2-City.mmdb") {
		t.Errorf("download mode failed: %v, %s", m2, p2)
	}

	m3, _ := decideMode("", "/data", "")
	if m3 != modeDisabled {
		t.Errorf("disabled mode failed: %v", m3)
	}

	// dataDirFor
	d1 := dataDirFor("sqlite", "file:/var/app/octarq.db?_pragma=1")
	if d1 != "/var/app" {
		t.Errorf("dataDirFor sqlite failed: %s", d1)
	}

	d2 := dataDirFor("postgres", "postgres://user:pass@host/db")
	if d2 != "." {
		t.Errorf("dataDirFor postgres expected '.', got %s", d2)
	}
}

func TestHelpers(t *testing.T) {
	t.Parallel()

	// redactKey
	err := errors.New("failed request to http://example.com?license_key=secretkey123&foo=bar")
	redacted := redactKey(err, "secretkey123")
	if redacted == nil || bytes.Contains([]byte(redacted.Error()), []byte("secretkey123")) {
		t.Errorf("redactKey failed to redact key: %v", redacted)
	}

	// firstN
	s1 := firstN("hello world", 5)
	if s1 != "hello…" {
		t.Errorf("firstN failed: %s", s1)
	}
	s2 := firstN("hi", 5)
	if s2 != "hi" {
		t.Errorf("firstN failed: %s", s2)
	}

	// extractMMDB non-gzip error
	if err := extractMMDB(bytes.NewReader([]byte("not gzip")), &bytes.Buffer{}); err == nil {
		t.Error("expected error extracting non-gzip data")
	}
}

func TestDownloadFlow(t *testing.T) {
	t.Parallel()

	// Create a dummy .tar.gz containing a dummy .mmdb file
	var tarBuf bytes.Buffer
	gw := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gw)

	dummyMMDB := []byte("dummy mmdb content")
	hdr := &tar.Header{
		Name:     "GeoLite2-City_20230101/GeoLite2-City.mmdb",
		Mode:     0600,
		Size:     int64(len(dummyMMDB)),
		Typeflag: tar.TypeReg,
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(dummyMMDB)
	_ = tw.Close()
	_ = gw.Close()

	tarBytes := tarBuf.Bytes()
	h := sha256.Sum256(tarBytes)
	shaHex := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suffix := r.URL.Query().Get("suffix")
		if suffix == "tar.gz.sha256" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf("%s  GeoLite2-City.tar.gz\n", shaHex)))
			return
		}
		if suffix == "tar.gz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(tarBytes)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tempDir := t.TempDir()
	path, err := download(context.Background(), downloadOptions{
		licenseKey: "testkey",
		dir:        tempDir,
		baseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if path != filepath.Join(tempDir, "GeoLite2-City.mmdb") {
		t.Errorf("unexpected download path: %s", path)
	}
}
