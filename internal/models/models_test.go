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

func TestEffectiveHosts(t *testing.T) {
	sub := Domain{Name: "example.com", LinkHosts: StringList{"go.example.com"}, MailHosts: StringList{"mail.example.com"}}
	if got := sub.EffectiveLinkHosts(); len(got) != 1 || got[0] != "go.example.com" {
		t.Errorf("link hosts: %v", got)
	}
	if got := sub.EffectiveMailHosts(); len(got) != 1 || got[0] != "mail.example.com" {
		t.Errorf("mail hosts: %v", got)
	}
	off := Domain{Name: "example.com"}
	if got := off.EffectiveLinkHosts(); got != nil {
		t.Errorf("no hosts should be nil: %v", got)
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
