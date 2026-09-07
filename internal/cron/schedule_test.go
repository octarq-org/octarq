package cron

import (
	"testing"
	"time"
)

func TestParseStandard_Valid5Fields(t *testing.T) {
	tests := []struct {
		spec string
	}{
		{"* * * * *"},
		{"0 * * * *"},
		{"0 0 * * *"},
		{"*/5 * * * *"},
		{"0,15,30,45 * * * *"},
		{"0 9-17 * * 1-5"},
		{"0 0 1 JAN-DEC *"},
		{"0 0 * * SUN,MON,TUE,WED,THU,FRI,SAT"},
		{"0 0 * * 7"}, // 7 is Sunday
		{"1-30/5 * * * *"},
		{"* * ? * *"},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			s, err := ParseStandard(tt.spec)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.spec, err)
			}
			if s == nil {
				t.Fatalf("expected non-nil schedule for %q", tt.spec)
			}
		})
	}
}

func TestParseStandard_Valid6Fields(t *testing.T) {
	tests := []struct {
		spec string
	}{
		{"* * * * * *"},
		{"*/10 * * * * *"},
		{"0 0 0 * * *"},
		{"5-30/5 * * * * *"},
		{"0 0 12 1 1 ?"},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			s, err := ParseStandard(tt.spec)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.spec, err)
			}
			if s == nil {
				t.Fatalf("expected non-nil schedule for %q", tt.spec)
			}
		})
	}
}

func TestParseStandard_Descriptors(t *testing.T) {
	tests := []struct {
		spec string
	}{
		{"@hourly"},
		{"@daily"},
		{"@midnight"},
		{"@weekly"},
		{"@monthly"},
		{"@yearly"},
		{"@annually"},
		{"@every 5s"},
		{"@every 1h30m"},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			s, err := ParseStandard(tt.spec)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.spec, err)
			}
			now := time.Now()
			next := s.Next(now)
			if next.IsZero() || !next.After(now) {
				t.Fatalf("expected next run after now, got %v for %q", next, tt.spec)
			}
		})
	}
}

func TestParseStandard_Invalid(t *testing.T) {
	tests := []struct {
		name string
		spec string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"too few fields", "* * *"},
		{"too many fields", "* * * * * * *"},
		{"unknown descriptor", "@unknown"},
		{"invalid every duration", "@every invalid"},
		{"negative every duration", "@every -5s"},
		{"zero every duration", "@every 0s"},
		{"invalid minute", "60 * * * *"},
		{"invalid hour", "* 24 * * *"},
		{"invalid dom", "* * 32 * *"},
		{"invalid month", "* * * 13 *"},
		{"invalid dow", "* * * * 8"},
		{"invalid name", "* * * FOO *"},
		{"range start > end", "10-5 * * * *"},
		{"invalid step", "*/0 * * * *"},
		{"empty part in list", "1,,2 * * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := ParseStandard(tt.spec)
			if err == nil {
				t.Fatalf("expected error for %q, got schedule %v", tt.spec, s)
			}
		})
	}
}

func TestCronSchedule_NextCalculations(t *testing.T) {
	loc := time.UTC
	base := time.Date(2026, 9, 6, 12, 0, 0, 0, loc)

	// Hourly at min 0
	s, err := ParseStandard("0 * * * *")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	next := s.Next(base)
	expected := time.Date(2026, 9, 6, 13, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}

	// Daily at midnight
	s, _ = ParseStandard("0 0 * * *")
	next = s.Next(base)
	expected = time.Date(2026, 9, 7, 0, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}

	// Seconds matching
	s, _ = ParseStandard("30 * * * * *")
	next = s.Next(base)
	expected = time.Date(2026, 9, 6, 12, 0, 30, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}

	// Specific month and day
	s, _ = ParseStandard("0 0 1 1 *")
	next = s.Next(base)
	expected = time.Date(2027, 1, 1, 0, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}

	// Every schedule
	every := EverySchedule{Duration: -1}
	if !every.Next(base).IsZero() {
		t.Fatalf("expected zero for non-positive duration")
	}
}

func TestCronSchedule_DayMatchingVixie(t *testing.T) {
	loc := time.UTC
	base := time.Date(2026, 9, 6, 0, 0, 0, 0, loc) // 2026-09-06 is Sunday

	// Restricted DOM only (the 10th of every month)
	s, _ := ParseStandard("0 0 10 * *")
	next := s.Next(base)
	expected := time.Date(2026, 9, 10, 0, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}

	// Restricted DOW only (Monday = 1)
	s, _ = ParseStandard("0 0 * * 1")
	next = s.Next(base)
	expected = time.Date(2026, 9, 7, 0, 0, 0, 0, loc) // Monday Sept 7
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}

	// Both restricted: Vixie cron union (10th of month OR Monday)
	// Base is Sept 6. Sept 7 is Monday (matches DOW), Sept 10 is Thursday (matches DOM)
	// Earliest is Sept 7.
	s, _ = ParseStandard("0 0 10 * 1")
	next = s.Next(base)
	expected = time.Date(2026, 9, 7, 0, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}
}
