package links

import (
	"testing"
)

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
