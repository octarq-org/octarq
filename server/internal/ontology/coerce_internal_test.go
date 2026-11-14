package ontology

import (
	"testing"
)

// TestToInt64RefusesAFloatThatLostPrecision pins the 2^53 bound. A float64
// cannot tell two integers apart past that point, so a value arriving there is
// not the value the caller sent and must not be stored as if it were.
func TestToInt64RefusesAFloatThatLostPrecision(t *testing.T) {
	f := Field{Name: "seats", Type: TypeInteger}

	got, err := toInt64(f, float64(1<<53))
	if err != nil {
		t.Fatalf("toInt64(2^53) = %v, want it accepted: it is exactly representable", err)
	}
	if got != int64(1<<53) {
		t.Fatalf("toInt64(2^53) = %v, want %d", got, int64(1<<53))
	}
	if _, err := toInt64(f, float64(1<<53)*2); err == nil {
		t.Fatal("toInt64(2^54) = nil, want a range refusal")
	}
}
