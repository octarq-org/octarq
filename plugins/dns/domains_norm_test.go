package dns

import (
	"errors"
	"testing"
)

func TestNormalizeHostUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"  HTTPS://MYDOMAIN.COM/path/to/page:8080. ", "mydomain.com"},
		{"http://sub.domain.com./", "sub.domain.com"},
		{"example.com:443", "example.com"},
		{"EXAMPLE.COM.", "example.com"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		got := normalizeHost(tt.input)
		if got != tt.want {
			t.Errorf("normalizeHost(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeHostsUnit(t *testing.T) {
	t.Parallel()

	tTrue := true
	fFalse := false

	entries := []hostEntry{
		{Host: "  HTTPS://A.COM/path ", Enabled: &tTrue},
		{Host: "a.com", Enabled: &fFalse}, // duplicate, should be skipped
		{Host: "B.COM:80", Enabled: &fFalse},
		{Host: "   ", Enabled: &tTrue}, // empty, skipped
	}

	out := normalizeHosts(entries)
	if len(out) != 2 {
		t.Fatalf("expected 2 host entries, got %d", len(out))
	}

	if out[0].Host != "a.com" || !out[0].Enabled {
		t.Errorf("entry 0 mismatch: %+v", out[0])
	}
	if out[1].Host != "b.com" || out[1].Enabled {
		t.Errorf("entry 1 mismatch: %+v", out[1])
	}
}

func TestProviderErrUnit(t *testing.T) {
	t.Parallel()

	p := &Plugin{}
	err := p.providerErr("create domain", errors.New("upstream timeout"))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() != "create domain: upstream timeout" {
		t.Errorf("unexpected error string: %v", err.Error())
	}
}
