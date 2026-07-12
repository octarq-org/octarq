package geo

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// cachedFileName is where the auto-downloaded database lands, inside the
	// data dir (the directory holding the sqlite database).
	cachedFileName = "GeoLite2-City.mmdb"

	editionID          = "GeoLite2-City"
	defaultBaseURL     = "https://download.maxmind.com"
	maxCacheAge        = 60 * 24 * time.Hour // past this, suggest a re-download
	retryDelay         = 30 * time.Second
	maxArchiveBytes    = 512 << 20 // tar.gz stream cap (the archive is ~40 MB)
	maxDBBytes         = 512 << 20 // extracted mmdb cap (~60 MB in practice)
	defaultHTTPTimeout = 5 * time.Minute
)

type mode string

const (
	modeManual   mode = "manual"
	modeCached   mode = "cached"
	modeDownload mode = "auto-download"
	modeDisabled mode = "disabled"
)

// decideMode picks the geo database source. Precedence: manual path >
// existing file in the data dir > auto-download with a license key > disabled.
// The returned path is the effective mmdb location (the download destination
// for modeDownload; empty for modeDisabled).
func decideMode(manualPath, dataDir, licenseKey string) (mode, string) {
	if manualPath != "" {
		return modeManual, manualPath
	}
	cached := filepath.Join(dataDir, cachedFileName)
	if fi, err := os.Stat(cached); err == nil && fi.Mode().IsRegular() {
		return modeCached, cached
	}
	if licenseKey != "" {
		return modeDownload, cached
	}
	return modeDisabled, ""
}

// dataDirFromEnv mirrors how config resolves the sqlite location: the geo
// database lives next to the sqlite file (e.g. /data in the Docker image).
// config.Load() has already merged .env into the process environment, so
// plain env reads see the same values.
func dataDirFromEnv() string {
	driver := os.Getenv("OCTARQ_DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	dsn := os.Getenv("OCTARQ_DB_DSN")
	if dsn == "" {
		dsn = "octarq.db"
	}
	return dataDirFor(driver, dsn)
}

// dataDirFor derives the state directory from the database configuration.
// Postgres deployments have no local db file, so state lands in the cwd.
func dataDirFor(driver, dsn string) string {
	if driver != "sqlite" {
		return "."
	}
	dsn = strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(dsn, '?'); i >= 0 { // sqlite DSNs may carry ?_pragma=… params
		dsn = dsn[:i]
	}
	if d := filepath.Dir(dsn); d != "" {
		return d
	}
	return "."
}

// downloadOptions parameterizes download; baseURL/client are injectable so
// tests can point at an httptest server instead of MaxMind.
type downloadOptions struct {
	licenseKey string
	dir        string       // destination directory for the mmdb
	baseURL    string       // defaults to MaxMind
	client     *http.Client // defaults to a 5-minute-timeout client
}

// download fetches the GeoLite2-City tar.gz from MaxMind, verifies it against
// the published sha256, extracts the .mmdb, and installs it atomically at
// <dir>/GeoLite2-City.mmdb. Returns the installed path.
func download(ctx context.Context, opts downloadOptions) (string, error) {
	base := opts.baseURL
	if base == "" {
		base = defaultBaseURL
	}
	client := opts.client
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}

	wantSum, err := fetchChecksum(ctx, client, base, opts.licenseKey)
	if err != nil {
		return "", err
	}

	// Stream the archive to a temp file while hashing, then verify before
	// extracting anything from it.
	tarTmp, err := os.CreateTemp(opts.dir, ".geolite2-*.tar.gz")
	if err != nil {
		return "", err
	}
	defer func() {
		tarTmp.Close()
		os.Remove(tarTmp.Name())
	}()
	gotSum, err := fetchArchive(ctx, client, base, opts.licenseKey, tarTmp)
	if err != nil {
		return "", err
	}
	if gotSum != wantSum {
		return "", fmt.Errorf("geoip download: sha256 mismatch (got %s, want %s)", gotSum, wantSum)
	}
	if _, err := tarTmp.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	dbTmp, err := os.CreateTemp(opts.dir, ".geolite2-*.mmdb")
	if err != nil {
		return "", err
	}
	defer os.Remove(dbTmp.Name()) // no-op after the successful rename
	if err := extractMMDB(tarTmp, dbTmp); err != nil {
		dbTmp.Close()
		return "", err
	}
	if err := dbTmp.Close(); err != nil {
		return "", err
	}

	dest := filepath.Join(opts.dir, cachedFileName)
	if err := os.Rename(dbTmp.Name(), dest); err != nil {
		return "", err
	}
	return dest, nil
}

// fetchChecksum retrieves and parses the "<hex>  <filename>" sha256 line
// MaxMind publishes next to the archive.
func fetchChecksum(ctx context.Context, client *http.Client, base, key string) (string, error) {
	body, err := fetch(ctx, client, base, key, "tar.gz.sha256")
	if err != nil {
		return "", err
	}
	defer body.Close()
	line, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil {
		return "", redactKey(err, key)
	}
	fields := strings.Fields(string(line))
	if len(fields) == 0 {
		return "", errors.New("geoip download: empty sha256 response")
	}
	sum := strings.ToLower(fields[0])
	if len(sum) != sha256.Size*2 {
		return "", fmt.Errorf("geoip download: malformed sha256 response %q", firstN(string(line), 80))
	}
	if _, err := hex.DecodeString(sum); err != nil {
		return "", fmt.Errorf("geoip download: malformed sha256 response %q", firstN(string(line), 80))
	}
	return sum, nil
}

// fetchArchive streams the tar.gz into w and returns its sha256 (hex).
func fetchArchive(ctx context.Context, client *http.Client, base, key string, w io.Writer) (string, error) {
	body, err := fetch(ctx, client, base, key, "tar.gz")
	if err != nil {
		return "", err
	}
	defer body.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(body, maxArchiveBytes)); err != nil {
		return "", redactKey(err, key)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fetch performs the MaxMind geoip_download request for the given suffix and
// returns the response body on HTTP 200. Errors never echo the license key.
func fetch(ctx context.Context, client *http.Client, base, key, suffix string) (io.ReadCloser, error) {
	u := base + "/app/geoip_download?edition_id=" + editionID +
		"&license_key=" + url.QueryEscape(key) + "&suffix=" + suffix
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, redactKey(err, key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, redactKey(err, key)
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		resp.Body.Close()
		return nil, fmt.Errorf("geoip download (%s): HTTP %d from MaxMind (check OCTARQ_MAXMIND_LICENSE_KEY): %s",
			suffix, resp.StatusCode, firstN(strings.TrimSpace(string(snippet)), 120))
	}
	return resp.Body, nil
}

// extractMMDB scans the tar.gz stream for the first .mmdb entry (MaxMind ships
// it inside a dated directory) and copies it to w.
func extractMMDB(r io.Reader, w io.Writer) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("geoip download: not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("geoip download: no .mmdb file found in archive")
		}
		if err != nil {
			return fmt.Errorf("geoip download: reading archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || !strings.HasSuffix(hdr.Name, ".mmdb") {
			continue
		}
		if _, err := io.Copy(w, io.LimitReader(tr, maxDBBytes)); err != nil {
			return fmt.Errorf("geoip download: extracting %s: %w", filepath.Base(hdr.Name), err)
		}
		return nil
	}
}

// redactKey scrubs the license key from an error (net/http errors embed the
// full request URL, which carries the key as a query parameter).
func redactKey(err error, key string) error {
	if err == nil || key == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), key, "***")
	msg = strings.ReplaceAll(msg, url.QueryEscape(key), "***")
	return errors.New(msg)
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
