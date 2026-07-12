// Package geo resolves a client IP into country/city (optional, via a MaxMind
// GeoLite2 mmdb) and parses a User-Agent into device/browser/os.
package geo

import (
	"context"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/mileusna/useragent"
	geoip2 "github.com/oschwald/geoip2-golang"
)

// Resolver looks up geographic info; nil-safe when no database is configured.
// The underlying reader is guarded by a mutex so a background auto-download
// can hot-load the database after startup (see Open).
type Resolver struct {
	mu     sync.RWMutex
	db     *geoip2.Reader
	closed bool
	cancel context.CancelFunc // stops a pending background download, if any
}

// Open builds the resolver for the given OCTARQ_GEOIP_DB value.
//
// Precedence (one mode line is logged saying which is active):
//  1. path != "" (manual OCTARQ_GEOIP_DB): load that file; an error is
//     returned to the caller (geo then stays disabled).
//  2. An already-downloaded GeoLite2-City.mmdb in the data dir (next to the
//     sqlite database): load it, skip downloading. If it is older than ~60
//     days a warning suggests deleting it to trigger a fresh download.
//  3. OCTARQ_MAXMIND_LICENSE_KEY set: return a no-op resolver immediately and
//     auto-download GeoLite2-City from MaxMind in a background goroutine, then
//     hot-load it. Geo lookups degrade gracefully (empty results) until ready.
//     Failures log a warning (one retry) and leave geo disabled — never crash.
//  4. Otherwise geo is disabled.
//
// The file is read into memory rather than memory-mapped: geoip2.Open uses
// mmap, which fails with ENODEV ("no such device") on filesystems that don't
// support it (some Docker bind mounts / overlay / network volumes). Reading
// the bytes avoids that and works everywhere.
//
// The license key and data dir are read from the environment rather than
// config.Config so the geo package stays self-contained: config.Load() has
// already merged .env into the process environment by the time Open runs.
func Open(path string) (*Resolver, error) {
	if path != "" {
		r := &Resolver{}
		if err := r.Load(path); err != nil {
			return nil, err
		}
		slog.Info("geoip: using configured database", "mode", "manual", "path", path)
		return r, nil
	}
	return openAuto(dataDirFromEnv(), os.Getenv("OCTARQ_MAXMIND_LICENSE_KEY"))
}

// openAuto handles precedence steps 2–4 (cached file, auto-download, disabled).
func openAuto(dataDir, licenseKey string) (*Resolver, error) {
	r := &Resolver{}
	mode, cached := decideMode("", dataDir, licenseKey)

	if mode == modeCached {
		err := r.Load(cached)
		if err == nil {
			slog.Info("geoip: using downloaded database", "mode", "cached", "path", cached)
			if fi, statErr := os.Stat(cached); statErr == nil {
				if age := time.Since(fi.ModTime()); age > maxCacheAge {
					slog.Warn("geoip: cached database is stale; delete it to trigger a fresh auto-download on next start",
						"path", cached, "age_days", int(age.Hours()/24))
				}
			}
			return r, nil
		}
		// Corrupt/truncated file (e.g. an interrupted download from an old
		// run): fall through to re-download when a key is available.
		slog.Warn("geoip: cached database unusable, ignoring it", "path", cached, "err", err)
		mode = modeDownload
		if licenseKey == "" {
			mode = modeDisabled
		}
	}

	switch mode {
	case modeDownload:
		slog.Info("geoip: auto-downloading GeoLite2-City in the background", "mode", "auto-download", "dest", cached)
		ctx, cancel := context.WithCancel(context.Background())
		r.cancel = cancel
		go r.autoDownload(ctx, dataDir, licenseKey)
	default:
		slog.Info("geoip disabled: set OCTARQ_GEOIP_DB to an mmdb path, or set OCTARQ_MAXMIND_LICENSE_KEY to auto-download GeoLite2-City", "mode", "disabled")
	}
	return r, nil
}

// autoDownload fetches the database (one retry after a short delay) and
// hot-loads it into the resolver. Runs in a background goroutine; all failure
// paths log a warning and leave geo disabled.
func (r *Resolver) autoDownload(ctx context.Context, dataDir, licenseKey string) {
	opts := downloadOptions{licenseKey: licenseKey, dir: dataDir}
	path, err := download(ctx, opts)
	if err != nil {
		slog.Warn("geoip: auto-download failed, retrying once", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
		if path, err = download(ctx, opts); err != nil {
			slog.Warn("geoip: auto-download failed, geo lookups stay disabled", "err", err)
			return
		}
	}
	if err := r.Load(path); err != nil {
		slog.Warn("geoip: downloaded database failed to load", "path", path, "err", err)
		return
	}
	slog.Info("geoip: database downloaded and loaded", "path", path)
}

// Load reads the mmdb at path and installs it, replacing (and closing) any
// previously loaded database. Safe to call concurrently with Locate.
func (r *Resolver) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	db, err := geoip2.FromBytes(data)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		_ = db.Close()
		return nil
	}
	if r.db != nil {
		_ = r.db.Close()
	}
	r.db = db
	return nil
}

// Country, Region, and City for the given IP; empty strings when unavailable.
func (r *Resolver) Locate(ip string) (country, region, city string) {
	if r == nil {
		return "", "", ""
	}
	r.mu.RLock()
	db := r.db
	defer r.mu.RUnlock()
	if db == nil {
		return "", "", ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", "", ""
	}
	rec, err := db.City(parsed)
	if err != nil {
		return "", "", ""
	}
	country = rec.Country.IsoCode
	if len(rec.Subdivisions) > 0 {
		if name, ok := rec.Subdivisions[0].Names["en"]; ok {
			region = name
		} else {
			region = rec.Subdivisions[0].IsoCode
		}
	}
	if name, ok := rec.City.Names["en"]; ok {
		city = name
	}
	return country, region, city
}

func (r *Resolver) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.cancel != nil {
		r.cancel()
	}
	if r.db != nil {
		_ = r.db.Close()
		r.db = nil
	}
}

// UAInfo is the parsed device/browser/os breakdown.
type UAInfo struct {
	Device  string
	Browser string
	OS      string
}

// ParseUA classifies a User-Agent string.
func ParseUA(ua string) UAInfo {
	p := useragent.Parse(ua)
	device := "desktop"
	switch {
	case p.Mobile:
		device = "mobile"
	case p.Tablet:
		device = "tablet"
	case p.Bot:
		device = "bot"
	}
	return UAInfo{Device: device, Browser: p.Name, OS: p.OS}
}
