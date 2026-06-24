// Package geo resolves a client IP into country/city (optional, via a MaxMind
// GeoLite2 mmdb) and parses a User-Agent into device/browser/os.
package geo

import (
	"net"
	"os"

	"github.com/mileusna/useragent"
	geoip2 "github.com/oschwald/geoip2-golang"
)

// Resolver looks up geographic info; nil-safe when no database is configured.
type Resolver struct {
	db *geoip2.Reader
}

// Open loads the mmdb at path. An empty path yields a no-op resolver.
//
// The file is read into memory rather than memory-mapped: geoip2.Open uses
// mmap, which fails with ENODEV ("no such device") on filesystems that don't
// support it (some Docker bind mounts / overlay / network volumes). Reading
// the bytes avoids that and works everywhere.
func Open(path string) (*Resolver, error) {
	if path == "" {
		return &Resolver{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r, err := geoip2.FromBytes(data)
	if err != nil {
		return nil, err
	}
	return &Resolver{db: r}, nil
}

// Country and City for the given IP; empty strings when unavailable.
func (r *Resolver) Locate(ip string) (country, city string) {
	if r == nil || r.db == nil {
		return "", ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", ""
	}
	rec, err := r.db.City(parsed)
	if err != nil {
		return "", ""
	}
	country = rec.Country.IsoCode
	if name, ok := rec.City.Names["en"]; ok {
		city = name
	}
	return country, city
}

func (r *Resolver) Close() {
	if r != nil && r.db != nil {
		_ = r.db.Close()
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
