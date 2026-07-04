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
	sub := Domain{
		Name:      "example.com",
		LinkHosts: HostList{{Host: "go.example.com", Enabled: true}, {Host: "off.example.com", Enabled: false}},
		MailHosts: HostList{{Host: "mail.example.com", Enabled: true}},
	}
	if got := sub.EffectiveLinkHosts(); len(got) != 1 || got[0] != "go.example.com" {
		t.Errorf("enabled link hosts only: %v", got)
	}
	if got := sub.EffectiveMailHosts(); len(got) != 1 || got[0] != "mail.example.com" {
		t.Errorf("mail hosts: %v", got)
	}
	// Blocks: a disabled-only host blocks; an enabled or unlisted host does not.
	if !sub.LinkHosts.Blocks("off.example.com") {
		t.Error("disabled host should block")
	}
	if sub.LinkHosts.Blocks("go.example.com") {
		t.Error("enabled host should not block")
	}
	if sub.LinkHosts.Blocks("unknown.example.com") {
		t.Error("unlisted host should not block")
	}
}

func TestHostListLegacyScan(t *testing.T) {
	var l HostList
	if err := l.Scan(`["go.example.com","s.example.com"]`); err != nil {
		t.Fatalf("legacy scan: %v", err)
	}
	if len(l) != 2 || !l[0].Enabled || l[0].Host != "go.example.com" {
		t.Errorf("legacy []string not upgraded: %+v", l)
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

func TestRoutingRulesRoundTrip(t *testing.T) {
	cases := []RoutingRules{
		nil,
		{},
		{{Type: "geo", Match: "US", Target: "https://us"}},
	}
	for _, in := range cases {
		v, err := in.Value()
		if err != nil {
			t.Fatalf("Value(%v): %v", in, err)
		}
		var out RoutingRules
		if err := out.Scan(v); err != nil {
			t.Fatalf("Scan(%v): %v", v, err)
		}
		if len(out) != len(in) {
			t.Errorf("len mismatch: in=%v out=%v", in, out)
			continue
		}
		if len(in) > 0 && (out[0].Type != in[0].Type || out[0].Match != in[0].Match || out[0].Target != in[0].Target) {
			t.Errorf("round-trip mismatch: in=%v out=%v", in, out)
		}
	}

	// Test scan invalid type
	var out RoutingRules
	if err := out.Scan(123); err == nil {
		t.Error("expected error scanning int, got nil")
	}
}

func TestAllModels(t *testing.T) {
	list := AllModels()
	if len(list) == 0 {
		t.Error("expected models list to be non-empty")
	}
}
