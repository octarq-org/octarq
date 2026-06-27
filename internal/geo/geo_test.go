package geo

import (
	"testing"
)

func TestGeo(t *testing.T) {
	// 1. Open with empty path
	r, err := Open("")
	if err != nil {
		t.Fatalf("expected no error opening empty path, got %v", err)
	}

	// 2. Locate with empty resolver/db
	country, region, city := r.Locate("127.0.0.1")
	if country != "" || region != "" || city != "" {
		t.Errorf("expected empty results for empty resolver, got: %q, %q, %q", country, region, city)
	}

	country, region, city = r.Locate("invalid-ip")
	if country != "" || region != "" || city != "" {
		t.Errorf("expected empty results for invalid IP, got: %q, %q, %q", country, region, city)
	}

	// 3. Close is safe
	r.Close()

	// 4. ParseUA tests
	cases := []struct {
		ua      string
		device  string
		browser string
		os      string
	}{
		{
			ua:      "Mozilla/5.0 (iPhone; CPU iPhone OS 16_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Mobile/15E148 Safari/604.1",
			device:  "mobile",
			browser: "Safari",
			os:      "iOS",
		},
		{
			ua:      "Mozilla/5.0 (iPad; CPU OS 16_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Mobile/15E148 Safari/604.1",
			device:  "tablet",
			browser: "Safari",
			os:      "iOS",
		},
		{
			ua:      "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			device:  "bot",
			browser: "Googlebot",
			os:      "",
		},
		{
			ua:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			device:  "desktop",
			browser: "Chrome",
			os:      "macOS",
		},
	}

	for _, c := range cases {
		info := ParseUA(c.ua)
		if info.Device != c.device {
			t.Errorf("ParseUA(%q) device = %q, want %q", c.ua, info.Device, c.device)
		}
		if info.Browser != c.browser {
			t.Errorf("ParseUA(%q) browser = %q, want %q", c.ua, info.Browser, c.browser)
		}
		if info.OS != c.os {
			t.Errorf("ParseUA(%q) os = %q, want %q", c.ua, info.OS, c.os)
		}
	}
}
