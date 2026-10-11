package cron_test

import (
	"context"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/cron"
)

func TestCronCharacterizationExpressionParsing(t *testing.T) {
	// Valid standard 5-part expressions
	validSpecs := []string{
		"0 * * * *",
		"*/5 * * * *",
		"0 0 * * *",
		"0 0 1 * *",
		"30 4 * * 1-5",
	}

	now := time.Now()
	for _, spec := range validSpecs {
		sched, err := cron.ParseStandard(spec)
		if err != nil {
			t.Fatalf("expected spec %q to parse cleanly, got error: %v", spec, err)
		}
		next := sched.Next(now)
		if !next.After(now) {
			t.Fatalf("expected next execution %v to be after now %v for spec %q", next, now, spec)
		}
	}

	// Invalid expressions must fail closed
	invalidSpecs := []string{
		"",
		"invalid",
		"* * *",
		"60 * * * *",
		"* 25 * * *",
	}
	for _, spec := range invalidSpecs {
		_, err := cron.ParseStandard(spec)
		if err == nil {
			t.Fatalf("expected invalid spec %q to return error, got nil", spec)
		}
	}
}

func TestCronCharacterizationLocalGuard(t *testing.T) {
	guard := cron.NewLocalGuard()
	ctx := context.Background()

	// 1. First lock acquisition succeeds
	rel, ok, err := guard.Acquire(ctx, "test_job", time.Minute)
	if err != nil || !ok || rel == nil {
		t.Fatalf("expected initial lock acquisition to succeed, got ok=%v, err=%v", ok, err)
	}

	// 2. Concurrent acquisition for same job must fail
	_, ok2, err2 := guard.Acquire(ctx, "test_job", time.Minute)
	if err2 != nil || ok2 {
		t.Fatalf("expected concurrent lock to be rejected, got ok=%v, err=%v", ok2, err2)
	}

	// 3. Release lock
	rel()

	// 4. Lock can be acquired again
	rel3, ok3, err3 := guard.Acquire(ctx, "test_job", time.Minute)
	if err3 != nil || !ok3 || rel3 == nil {
		t.Fatalf("expected lock re-acquisition to succeed after release, got ok=%v, err=%v", ok3, err3)
	}
	rel3()
}
