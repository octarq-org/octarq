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
	a := HashToken("oct_abc")
	if a != HashToken("oct_abc") {
		t.Error("HashToken not deterministic")
	}
	if a == HashToken("oct_xyz") {
		t.Error("HashToken collided on different inputs")
	}
	// SHA-256 hex is 64 chars.
	if len(a) != 64 {
		t.Errorf("hash length = %d, want 64", len(a))
	}
}

func TestAllModels(t *testing.T) {
	list := AllModels()
	if len(list) == 0 {
		t.Error("expected models list to be non-empty")
	}
}
