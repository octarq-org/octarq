package geo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTarGz builds an in-memory tar.gz with the given name→content entries,
// mirroring MaxMind's layout (files inside a dated directory).
func makeTarGz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractMMDB(t *testing.T) {
	const want = "fake mmdb bytes"
	archive := makeTarGz(t, map[string]string{
		"GeoLite2-City_20260701/LICENSE.txt":        "license",
		"GeoLite2-City_20260701/COPYRIGHT.txt":      "copyright",
		"GeoLite2-City_20260701/GeoLite2-City.mmdb": want,
	})
	var out bytes.Buffer
	if err := extractMMDB(bytes.NewReader(archive), &out); err != nil {
		t.Fatalf("extractMMDB: %v", err)
	}
	if out.String() != want {
		t.Fatalf("extracted %q, want %q", out.String(), want)
	}
}

func TestExtractMMDBMissing(t *testing.T) {
	archive := makeTarGz(t, map[string]string{"GeoLite2-City_20260701/LICENSE.txt": "license"})
	err := extractMMDB(bytes.NewReader(archive), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no .mmdb") {
		t.Fatalf("want 'no .mmdb' error, got %v", err)
	}
}

func TestExtractMMDBNotGzip(t *testing.T) {
	if err := extractMMDB(strings.NewReader("not gzip"), &bytes.Buffer{}); err == nil {
		t.Fatal("want error for non-gzip input")
	}
}

// maxmindStub serves the two geoip_download suffixes like MaxMind does. The
// checksum served is sha256(archive) unless overridden via badSum.
func maxmindStub(t *testing.T, archive []byte, badSum bool) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(archive)
	sumHex := hex.EncodeToString(sum[:])
	if badSum {
		sumHex = strings.Repeat("0", 64)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/geoip_download" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("license_key") != "testkey" {
			http.Error(w, "Invalid license key", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("edition_id") != "GeoLite2-City" {
			http.Error(w, "unknown edition", http.StatusBadRequest)
			return
		}
		switch r.URL.Query().Get("suffix") {
		case "tar.gz":
			_, _ = w.Write(archive)
		case "tar.gz.sha256":
			fmt.Fprintf(w, "%s  GeoLite2-City_20260701.tar.gz\n", sumHex)
		default:
			http.Error(w, "bad suffix", http.StatusBadRequest)
		}
	}))
}

func TestDownloadSuccess(t *testing.T) {
	const want = "fake mmdb bytes"
	archive := makeTarGz(t, map[string]string{
		"GeoLite2-City_20260701/GeoLite2-City.mmdb": want,
		"GeoLite2-City_20260701/LICENSE.txt":        "license",
	})
	srv := maxmindStub(t, archive, false)
	defer srv.Close()

	dir := t.TempDir()
	path, err := download(context.Background(), downloadOptions{
		licenseKey: "testkey", dir: dir, baseURL: srv.URL, client: srv.Client(),
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if path != filepath.Join(dir, "GeoLite2-City.mmdb") {
		t.Fatalf("unexpected path %q", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("installed content %q, want %q", got, want)
	}
	// No temp litter left behind.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected only the mmdb in %s, found %d entries", dir, len(entries))
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	archive := makeTarGz(t, map[string]string{"x/GeoLite2-City.mmdb": "data"})
	srv := maxmindStub(t, archive, true)
	defer srv.Close()

	dir := t.TempDir()
	_, err := download(context.Background(), downloadOptions{
		licenseKey: "testkey", dir: dir, baseURL: srv.URL, client: srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want sha256 mismatch error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "GeoLite2-City.mmdb")); !os.IsNotExist(statErr) {
		t.Fatal("mmdb must not be installed on checksum mismatch")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no leftover files, found %d", len(entries))
	}
}

func TestDownloadBadKey(t *testing.T) {
	archive := makeTarGz(t, map[string]string{"x/GeoLite2-City.mmdb": "data"})
	srv := maxmindStub(t, archive, false)
	defer srv.Close()

	_, err := download(context.Background(), downloadOptions{
		licenseKey: "wrongkey", dir: t.TempDir(), baseURL: srv.URL, client: srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want HTTP 401 error, got %v", err)
	}
	if strings.Contains(err.Error(), "wrongkey") {
		t.Fatalf("error must not echo the license key: %v", err)
	}
}

func TestDownloadNetworkError(t *testing.T) {
	srv := maxmindStub(t, nil, false)
	srv.Close() // refuse connections

	_, err := download(context.Background(), downloadOptions{
		licenseKey: "secretkey123", dir: t.TempDir(), baseURL: srv.URL,
	})
	if err == nil {
		t.Fatal("want error when the server is unreachable")
	}
	if strings.Contains(err.Error(), "secretkey123") {
		t.Fatalf("error must not echo the license key: %v", err)
	}
}

func TestDecideModePrecedence(t *testing.T) {
	withCached := t.TempDir()
	cached := filepath.Join(withCached, cachedFileName)
	if err := os.WriteFile(cached, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()

	tests := []struct {
		name             string
		manual, dir, key string
		wantMode         mode
		wantPath         string
	}{
		{"manual wins over cached and key", "/etc/geo.mmdb", withCached, "key", modeManual, "/etc/geo.mmdb"},
		{"cached wins over key", "", withCached, "key", modeCached, cached},
		{"cached without key", "", withCached, "", modeCached, cached},
		{"key alone downloads", "", empty, "key", modeDownload, filepath.Join(empty, cachedFileName)},
		{"nothing disabled", "", empty, "", modeDisabled, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMode, gotPath := decideMode(tt.manual, tt.dir, tt.key)
			if gotMode != tt.wantMode || gotPath != tt.wantPath {
				t.Fatalf("decideMode(%q,%q,%q) = (%s, %q), want (%s, %q)",
					tt.manual, tt.dir, tt.key, gotMode, gotPath, tt.wantMode, tt.wantPath)
			}
		})
	}
}

func TestDataDirFor(t *testing.T) {
	tests := []struct {
		driver, dsn, want string
	}{
		{"sqlite", "octarq.db", "."},
		{"sqlite", "/data/octarq.db", "/data"},
		{"sqlite", "file:/data/octarq.db?_pragma=busy_timeout(5000)", "/data"},
		{"postgres", "postgres://u:p@host/db", "."},
	}
	for _, tt := range tests {
		if got := dataDirFor(tt.driver, tt.dsn); got != tt.want {
			t.Errorf("dataDirFor(%q, %q) = %q, want %q", tt.driver, tt.dsn, got, tt.want)
		}
	}
}

// TestResolverLoadInvalid: a bad file errors out and the resolver keeps
// degrading gracefully (empty lookups), never panicking.
func TestResolverLoadInvalid(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.mmdb")
	if err := os.WriteFile(bad, []byte("not an mmdb"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Resolver{}
	if err := r.Load(bad); err == nil {
		t.Fatal("want error loading a corrupt mmdb")
	}
	if c, _, _ := r.Locate("8.8.8.8"); c != "" {
		t.Fatalf("expected empty country from unloaded resolver, got %q", c)
	}
	r.Close()
}
