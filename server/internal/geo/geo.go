package geo

import (
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/mileusna/useragent"
	geoip2 "github.com/oschwald/geoip2-golang"
)

type Resolver struct {
	mu     sync.RWMutex
	db     *geoip2.Reader
	closed bool
}

func Open(path string) (*Resolver, error) {
	r := &Resolver{}
	if path == "" {
		slog.Info("geoip disabled: set OCTARQ_GEOIP_DB to an mmdb path to enable geo lookups", "mode", "disabled")
		return r, nil
	}
	if err := r.Load(path); err != nil {
		return nil, err
	}
	slog.Info("geoip: using configured database", "mode", "manual", "path", path)
	return r, nil
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
	if r.db != nil {
		if closeErr := r.db.Close(); closeErr != nil {
			slog.Warn("geo: failed to close db during resolver teardown", "err", closeErr)
		}
		r.db = nil
	}
	r.mu.Unlock()
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
