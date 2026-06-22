package models

import (
	"reflect"
	"testing"
)

func TestStringListRoundTrip(t *testing.T) {
	cases := []StringList{nil, {}, {"go.example.com"}, {"go.example.com", "s.example.com"}}
	for _, in := range cases {
		v, err := in.Value()
		if err != nil {
			t.Fatalf("Value(%v): %v", in, err)
		}
		var out StringList
		if err := out.Scan(v); err != nil {
			t.Fatalf("Scan(%v): %v", v, err)
		}
		if len(out) != len(in) {
			t.Errorf("len mismatch: in=%v out=%v", in, out)
			continue
		}
		if len(in) > 0 && !reflect.DeepEqual([]string(in), []string(out)) {
			t.Errorf("round-trip mismatch: in=%v out=%v", in, out)
		}
	}
}

func TestEffectiveHostsFallback(t *testing.T) {
	apex := Domain{Name: "example.com", ForLink: true, ForMail: true}
	if got := apex.EffectiveLinkHosts(); len(got) != 1 || got[0] != "example.com" {
		t.Errorf("link apex fallback: %v", got)
	}
	if got := apex.EffectiveMailHosts(); len(got) != 1 || got[0] != "example.com" {
		t.Errorf("mail apex fallback: %v", got)
	}
	sub := Domain{Name: "example.com", ForLink: true, LinkHosts: StringList{"go.example.com"}}
	if got := sub.EffectiveLinkHosts(); len(got) != 1 || got[0] != "go.example.com" {
		t.Errorf("explicit link host: %v", got)
	}
	off := Domain{Name: "example.com"}
	if got := off.EffectiveLinkHosts(); got != nil {
		t.Errorf("disabled should be nil: %v", got)
	}
}

func TestHashTokenDeterministicAndDistinct(t *testing.T) {
	a := HashToken("led_abc")
	if a != HashToken("led_abc") {
		t.Error("HashToken not deterministic")
	}
	if a == HashToken("led_xyz") {
		t.Error("HashToken collided on different inputs")
	}
	// SHA-256 hex is 64 chars.
	if len(a) != 64 {
		t.Errorf("hash length = %d, want 64", len(a))
	}
}
