package api

import "testing"

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
