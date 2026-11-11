package plugin

import (
	"strings"
	"testing"
)

func TestRandomSlug(t *testing.T) {
	for _, l := range []int{0, 1, 6, 8, 16, 32} {
		s := RandomSlug(l)
		if len(s) != l {
			t.Fatalf("RandomSlug(%d) length = %d, want %d", l, len(s), l)
		}
		for _, ch := range s {
			if !strings.ContainsRune(slugAlphabet, ch) {
				t.Fatalf("RandomSlug contained unexpected rune %c", ch)
			}
		}
	}
}

func TestRandomSlugUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		s := RandomSlug(10)
		if seen[s] {
			t.Fatalf("RandomSlug collision detected: %s", s)
		}
		seen[s] = true
	}
}

func TestUsageMetricConstants(t *testing.T) {
	if MetricClicks != "clicks" {
		t.Errorf("MetricClicks = %q, want clicks", MetricClicks)
	}
	if MetricMailIn != "mailIn" {
		t.Errorf("MetricMailIn = %q, want mailIn", MetricMailIn)
	}
	if MetricMailOut != "mailOut" {
		t.Errorf("MetricMailOut = %q, want mailOut", MetricMailOut)
	}
	if MetricRawBytes != "mail.raw_bytes" {
		t.Errorf("MetricRawBytes = %q, want mail.raw_bytes", MetricRawBytes)
	}
}
