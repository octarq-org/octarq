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

type Resolver struct {
	mu     sync.RWMutex
	db     *geoip2.Reader
	closed bool
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

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
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.autoDownload(ctx, dataDir, licenseKey)
		}()
	default:
		slog.Info("geoip disabled: set OCTARQ_GEOIP_DB to an mmdb path, or set OCTARQ_MAXMIND_LICENSE_KEY to auto-download GeoLite2-City", "mode", "disabled")
	}
	return r, nil
}

func (r *Resolver) autoDownload(ctx context.Context, dataDir, licenseKey string) {
	opts := downloadOptions{licenseKey: licenseKey, dir: dataDir}
	path, err := downloadFn(ctx, opts)
	if err != nil {
		slog.Warn("geoip: auto-download failed, retrying once", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
		if path, err = downloadFn(ctx, opts); err != nil {
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
		if closeErr := db.Close(); closeErr != nil {
			slog.Warn("geo: failed to close parsed db on closed resolver", "err", closeErr)
		}
		return nil
	}
	if r.db != nil {
		if closeErr := r.db.Close(); closeErr != nil {
			slog.Warn("geo: failed to close previous db", "err", closeErr)
		}
	}
	r.db = db
	return nil
}

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
	r.closed = true
	if r.cancel != nil {
		r.cancel()
	}
	if r.db != nil {
		if closeErr := r.db.Close(); closeErr != nil {
			slog.Warn("geo: failed to close db during resolver teardown", "err", closeErr)
		}
		r.db = nil
	}
	r.mu.Unlock()
	r.wg.Wait()
}

type UAInfo struct {
	Device  string
	Browser string
	OS      string
}

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
