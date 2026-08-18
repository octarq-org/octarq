package readiness

import (
	"strings"
	"testing"

	"github.com/octarq-org/octarq/config"
)

func check(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	for _, c := range checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %q not in report: %v", id, checks)
	return Check{}
}

// TestEvaluateAllStates drives every capability state the report can express:
// degraded/ok on domains and mail, blocked registration when verification is
// required without a sender, the dev-vs-degraded secret-key split by domain
// registration, and the dev hardening row for unprovisioned instances.
func TestEvaluateAllStates(t *testing.T) {
	cfg := &config.Config{
		DBDriver:  "sqlite",
		DBDSN:     "/data/octarq.db",
		SecretKey: "short", // below MinSecretKeyLen: the dev/degraded split depends on it
	}

	t.Run("unprovisioned with short key and no domains", func(t *testing.T) {
		checks := Evaluate(cfg, false, false, false)

		if got := check(t, checks, "public-origin").Status; got != StatusDegraded {
			t.Errorf("public-origin = %q, want degraded", got)
		}
		if got := check(t, checks, "outbound-mail").Status; got != StatusDegraded {
			t.Errorf("outbound-mail = %q, want degraded", got)
		}
		if got := check(t, checks, "registration").Status; got != StatusOK {
			t.Errorf("registration = %q, want ok", got)
		}
		if got := check(t, checks, "secret-key"); got.Status != StatusDev {
			t.Errorf("secret-key = %q, want dev", got)
		}
		// The hardening row only appears for unprovisioned instances.
		if got := check(t, checks, "hardening").Status; got != StatusDev {
			t.Errorf("hardening = %q, want dev", got)
		}
		if db := check(t, checks, "database").Detail; !strings.Contains(db, "driver=sqlite dsn=/data/octarq.db") {
			t.Errorf("database detail = %q, want the sqlite DSN shown", db)
		}
	})

	t.Run("provisioned with long key and registered domain", func(t *testing.T) {
		prov := &config.Config{
			DBDriver:  "postgres",
			DBDSN:     "postgres://user:secret@db:5432/octarq",
			SecretKey: strings.Repeat("k", config.MinSecretKeyLen+4),
		}
		checks := Evaluate(prov, true, true, true)

		if got := check(t, checks, "public-origin").Status; got != StatusOK {
			t.Errorf("public-origin = %q, want ok", got)
		}
		if got := check(t, checks, "outbound-mail").Status; got != StatusOK {
			t.Errorf("outbound-mail = %q, want ok", got)
		}
		if got := check(t, checks, "registration").Status; got != StatusOK {
			t.Errorf("registration = %q, want ok", got)
		}
		if got := check(t, checks, "secret-key"); got.Status != StatusOK {
			t.Errorf("secret-key = %q, want ok", got)
		}
		if db := check(t, checks, "database").Detail; !strings.Contains(db, "withheld") {
			t.Errorf("database detail = %q, want the postgres DSN withheld", db)
		}
		for _, c := range checks {
			if c.ID == "hardening" {
				t.Error("provisioned instance still reports the hardening row")
			}
		}
	})

	t.Run("registration blocked without verified mail", func(t *testing.T) {
		checks := Evaluate(cfg, false, false, true)
		if got := check(t, checks, "registration").Status; got != StatusBlocked {
			t.Errorf("registration = %q, want blocked", got)
		}
		if detail := check(t, checks, "registration").Detail; !strings.Contains(detail, "broken") {
			t.Errorf("registration detail = %q, want the broken-flow note", detail)
		}
	})

	t.Run("registration disabled does not depend on sender", func(t *testing.T) {
		checks := Evaluate(cfg, false, false, false)
		if detail := check(t, checks, "registration").Detail; !strings.Contains(detail, "disabled") {
			t.Errorf("registration detail = %q, want the disabled email note", detail)
		}
	})

	t.Run("short secret with a registered domain is degraded not dev", func(t *testing.T) {
		short := &config.Config{DBDriver: "sqlite", DBDSN: "/x.db", SecretKey: "short"}
		checks := Evaluate(short, false, true, false)
		if got := check(t, checks, "secret-key").Status; got != StatusDegraded {
			t.Errorf("secret-key with domain = %q, want degraded", got)
		}
	})
}

// TestRedactDSN covers the three renderings: sqlite paths are shown, empty
// DSNs say not configured, everything else is withheld.
func TestRedactDSN(t *testing.T) {
	if got := redactDSN("sqlite", "/data/db.sqlite"); got != "/data/db.sqlite" {
		t.Errorf("sqlite DSN = %q", got)
	}
	if got := redactDSN("postgres", ""); got != "(not configured)" {
		t.Errorf("empty postgres DSN = %q", got)
	}
	if got := redactDSN("postgres", "postgres://u:p@h/db"); got != "(configured, withheld: may contain a password)" {
		t.Errorf("postgres DSN = %q", got)
	}
}
