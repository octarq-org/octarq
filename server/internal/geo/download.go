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
	cachedFileName = "GeoLite2-City.mmdb"

	editionID          = "GeoLite2-City"
	defaultBaseURL     = "https://download.maxmind.com"
	maxCacheAge        = 60 * 24 * time.Hour
	retryDelay         = 30 * time.Second
	maxArchiveBytes    = 512 << 20
	maxDBBytes         = 512 << 20
	defaultHTTPTimeout = 5 * time.Minute
)

type mode string

const (
	modeManual   mode = "manual"
	modeCached   mode = "cached"
	modeDownload mode = "auto-download"
	modeDisabled mode = "disabled"
)

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

func dataDirFor(driver, dsn string) string {
	if driver != "sqlite" {
		return "."
	}
	dsn = strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(dsn, '?'); i >= 0 {
		dsn = dsn[:i]
	}
	if d := filepath.Dir(dsn); d != "" {
		return d
	}
	return "."
}

type downloadOptions struct {
	licenseKey string
	dir        string
	baseURL    string
	client     *http.Client
}

var downloadFn = download

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
	defer os.Remove(dbTmp.Name())
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
		snippet, readErr := io.ReadAll(io.LimitReader(resp.Body, 200))
		if readErr != nil && len(snippet) == 0 {
			snippet = []byte(fmt.Sprintf("<error reading body: %v>", readErr))
		}
		resp.Body.Close()
		return nil, fmt.Errorf("geoip download (%s): HTTP %d from MaxMind (check OCTARQ_MAXMIND_LICENSE_KEY): %s",
			suffix, resp.StatusCode, firstN(strings.TrimSpace(string(snippet)), 120))
	}
	return resp.Body, nil
}

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
