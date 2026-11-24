package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/geo"
)

func TestLocationFromGeo(t *testing.T) {
	cases := []struct {
		name    string
		country string
		city    string
		want    string
	}{
		{"city and country", "US", "San Francisco", "San Francisco, US"},
		{"country only", "US", "", "US"},
		{"city only", "", "San Francisco", "San Francisco"},
		{"neither", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := locationFromGeo(c.country, c.city); got != c.want {
				t.Fatalf("locationFromGeo(%q, %q) = %q, want %q", c.country, c.city, got, c.want)
			}
		})
	}
}

func TestTrustedCountry(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"US", "US"},
		{"de", "DE"},
		{"", ""},
		{"XX", ""},
		{"USA", ""},
		{"U1", ""},
		{"  JP  ", "JP"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if c.header != "" {
			req.Header.Set("CF-IPCountry", c.header)
		}
		if got := trustedCountry(req); got != c.want {
			t.Errorf("trustedCountry(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

func TestSessionLocationUsesCountryHeaderOnlyWithTrustProxy(t *testing.T) {
	g, err := geo.Open("")
	if err != nil {
		t.Fatalf("geo.Open: %v", err)
	}
	defer g.Close()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("CF-IPCountry", "DE")

	trusted := &Handler{cfg: &config.Config{TrustProxy: true}, geo: g}
	if got := trusted.sessionLocation(req, "203.0.113.7"); got != "DE" {
		t.Errorf("trusted proxy location = %q, want DE", got)
	}

	untrusted := &Handler{cfg: &config.Config{TrustProxy: false}, geo: g}
	if got := untrusted.sessionLocation(req, "203.0.113.7"); got != "" {
		t.Errorf("untrusted proxy location = %q, want empty", got)
	}

	invalid := httptest.NewRequest(http.MethodGet, "/", nil)
	invalid.Header.Set("CF-IPCountry", "XX")
	if got := trusted.sessionLocation(invalid, "203.0.113.7"); got != "" {
		t.Errorf("XX header location = %q, want empty", got)
	}

	if got := trusted.sessionLocation(req, "127.0.0.1"); got != "Localhost" {
		t.Errorf("localhost location = %q, want Localhost", got)
	}
	if got := trusted.sessionLocation(req, "::1"); got != "Localhost" {
		t.Errorf("ipv6 localhost location = %q, want Localhost", got)
	}
}
