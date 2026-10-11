package geo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestCoverageLift_ExtractMMDB_WriteError(t *testing.T) {
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

	err := extractMMDB(bytes.NewReader(tarBuf.Bytes()), errWriter{})
	if err == nil {
		t.Fatal("expected error from extractMMDB when writer fails, got nil")
	}
}

func TestCoverageLift_AutoDownload_LoadFails(t *testing.T) {
	r := &Resolver{}
	origDownloadFn := downloadFn
	defer func() { downloadFn = origDownloadFn }()

	dummyFile := filepath.Join(t.TempDir(), "not-a-valid.mmdb")
	if err := os.WriteFile(dummyFile, []byte("invalid-mmdb-content"), 0600); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	downloadFn = func(ctx context.Context, opts downloadOptions) (string, error) {
		return dummyFile, nil
	}

	// Should attempt r.Load(dummyFile), fail, log warning, and return cleanly
	r.autoDownload(context.Background(), t.TempDir(), "test-license")
	if r.db != nil {
		t.Fatal("expected r.db to remain nil after failed load")
	}
}

func TestCoverageLift_SqliteDataDir(t *testing.T) {
	dir := dataDirFor("sqlite", "mydb.db?mode=memory")
	if dir != "." {
		t.Fatalf("expected ., got %q", dir)
	}
}

func TestCoverageLift_FetchArchive_WriteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-archive-bytes"))
	}))
	defer srv.Close()

	client := srv.Client()
	_, err := fetchArchive(context.Background(), client, srv.URL, "mykey", errWriter{})
	if err == nil {
		t.Fatal("expected error from fetchArchive with failing writer, got nil")
	}
}

func TestCoverageLift_FetchChecksum_ReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			if conn != nil {
				_ = conn.Close()
			}
		}
	}))
	defer srv.Close()

	client := srv.Client()
	_, err := fetchChecksum(context.Background(), client, srv.URL, "mykey")
	if err == nil {
		t.Fatal("expected error from fetchChecksum when connection drops, got nil")
	}
}

func TestCoverageLift_Download_EmptyBaseURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled context avoids hitting actual MaxMind network
	_, err := download(ctx, downloadOptions{
		baseURL:    "", // triggers defaultBaseURL branch
		licenseKey: "dummy",
	})
	if err == nil {
		t.Fatal("expected error from canceled context, got nil")
	}
}
