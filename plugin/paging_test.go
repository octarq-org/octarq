package plugin

import "testing"

// TestPageLimitClampsOverMaxToMax is the guard for the bug this helper exists
// to kill: an over-limit request must be clamped DOWN TO THE MAXIMUM, never
// dropped back to the default. Returning the default there hands a paginating
// client a short page it reads as the end of the collection.
func TestPageLimitClampsOverMaxToMax(t *testing.T) {
	if got := PageLimit(1000, 50, 500); got != 500 {
		t.Fatalf("PageLimit(1000, def=50, max=500) = %d, want 500 (the max, not the default)", got)
	}
	// One past the ceiling is the boundary the old code got wrong most quietly.
	if got := PageLimit(501, 50, 500); got != 500 {
		t.Fatalf("PageLimit(501, def=50, max=500) = %d, want 500", got)
	}
}

// TestPageLimitNeverReturnsLessThanDefaultForLargerRequest states the invariant
// behind the clamp: asking for MORE must never yield FEWER rows than asking for
// nothing at all.
func TestPageLimitNeverReturnsLessThanDefaultForLargerRequest(t *testing.T) {
	def, max := 50, 500
	base := PageLimit(0, def, max)
	for _, requested := range []int{1, 49, 50, 51, 499, 500, 501, 1000, 1 << 30} {
		if requested <= base {
			continue
		}
		if got := PageLimit(requested, def, max); got < base {
			t.Fatalf("PageLimit(%d) = %d, less than the no-parameter result %d", requested, got, base)
		}
	}
}

func TestPageLimitDefaultsAndPassthrough(t *testing.T) {
	cases := []struct {
		name           string
		requested      int
		def, max, want int
	}{
		{"absent", 0, 50, 500, 50},
		{"negative", -1, 50, 500, 50},
		{"under max", 200, 50, 500, 200},
		{"exactly max", 500, 50, 500, 500},
		{"exactly default", 50, 50, 500, 50},
		{"max below default is the real ceiling", 0, 100, 10, 10},
		{"max below default clamps request too", 90, 100, 10, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PageLimit(c.requested, c.def, c.max); got != c.want {
				t.Fatalf("PageLimit(%d, %d, %d) = %d, want %d", c.requested, c.def, c.max, got, c.want)
			}
		})
	}
}

func TestPageOffset(t *testing.T) {
	for _, c := range []struct{ in, want int }{{-5, 0}, {0, 0}, {1, 1}, {999, 999}} {
		if got := PageOffset(c.in); got != c.want {
			t.Fatalf("PageOffset(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
