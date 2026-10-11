package geo_test

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/geo"
)

func TestGeoCharacterizationNilResolverSafe(t *testing.T) {
	var r *geo.Resolver
	country, region, city := r.Locate("1.1.1.1")
	if country != "" || region != "" || city != "" {
		t.Fatalf("expected empty strings from nil resolver, got %q, %q, %q", country, region, city)
	}
}

func TestGeoCharacterizationUAParsing(t *testing.T) {
	uaStr := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	info := geo.ParseUA(uaStr)

	if info.Browser == "" {
		t.Fatal("expected non-empty browser")
	}
	if info.OS == "" {
		t.Fatal("expected non-empty OS")
	}
	if info.Device == "" {
		t.Fatal("expected non-empty Device")
	}
}
