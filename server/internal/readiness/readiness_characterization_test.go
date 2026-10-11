package readiness_test

import (
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/readiness"
)

func TestReadinessCharacterizationStatusConstants(t *testing.T) {
	if readiness.StatusOK != "ok" {
		t.Fatalf("StatusOK mismatch: %q", readiness.StatusOK)
	}
	if readiness.StatusDegraded != "degraded" {
		t.Fatalf("StatusDegraded mismatch: %q", readiness.StatusDegraded)
	}
	if readiness.StatusBlocked != "blocked" {
		t.Fatalf("StatusBlocked mismatch: %q", readiness.StatusBlocked)
	}
	if readiness.StatusDev != "dev" {
		t.Fatalf("StatusDev mismatch: %q", readiness.StatusDev)
	}
}

func TestReadinessCharacterizationBlockedRegistration(t *testing.T) {
	cfg := &config.Config{
		DBDriver:  "sqlite",
		DBDSN:     "test.db",
		SecretKey: "01234567890123456789012345678901",
	}

	// When requireEmailVerification=true but mailReady=false -> Registration MUST be StatusBlocked
	checks := readiness.Evaluate(cfg, false, false, true)
	var foundReg bool
	for _, c := range checks {
		if c.ID == "registration" {
			foundReg = true
			if c.Status != readiness.StatusBlocked {
				t.Fatalf("expected registration to be StatusBlocked, got %q", c.Status)
			}
		}
	}
	if !foundReg {
		t.Fatal("expected registration check in report")
	}
}
