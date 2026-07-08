package api

import (
	"testing"

	"github.com/octarq-org/octarq/internal/dnsprovider"
)

func TestValidateRecord(t *testing.T) {
	p := 10
	cases := []struct {
		name string
		rec  dnsprovider.Record
		ok   bool
	}{
		{"valid A", dnsprovider.Record{Type: "A", Content: "1.2.3.4"}, true},
		{"missing type", dnsprovider.Record{Content: "1.2.3.4"}, false},
		{"empty content", dnsprovider.Record{Type: "A", Content: ""}, false},
		{"MX without priority", dnsprovider.Record{Type: "MX", Content: "mx.x.com"}, false},
		{"MX with priority", dnsprovider.Record{Type: "MX", Content: "mx.x.com", Priority: &p}, true},
		{"lowercase mx without priority", dnsprovider.Record{Type: "mx", Content: "mx.x.com"}, false},
	}
	for _, c := range cases {
		msg := validateRecord(c.rec)
		if (msg == "") != c.ok {
			t.Errorf("%s: validateRecord = %q, wantOK=%v", c.name, msg, c.ok)
		}
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"  GO.Example.com/ ":    "go.example.com",
		"https://s.example.com": "s.example.com",
		"http://x.com/path":     "x.com",
		"a.com.":                "a.com",
	}
	for in, want := range cases {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}
