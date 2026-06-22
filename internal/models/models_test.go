package models

import "testing"

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
