package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/config"
)

// loadReadinessConfig builds a Config through the real config.Load so the
// report is exercised against the struct the running server actually holds,
// not a hand-filled literal that could drift from it.
func loadReadinessConfig(t *testing.T, env map[string]string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	base := map[string]string{
		"OCTARQ_DB_DRIVER":      "sqlite",
		"OCTARQ_DB_DSN":         filepath.Join(dir, "octarq.db"),
		"OCTARQ_SECRET_KEY":     "0123456789abcdef0123456789abcdef",
		"OCTARQ_ADMIN_PASSWORD": "pw",
	}
	for k, v := range env {
		base[k] = v
	}
	for k, v := range base {
		t.Setenv(k, v)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func reportText(lines []readinessLine) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.String())
		b.WriteString("\n")
	}
	return b.String()
}

// TestReadinessReportOmitsSecrets is the leak guard. The report is written to
// the operator's log stream, which is shipped, indexed and often shared in
// support threads; a KEK or admin password landing there is a compromise that
// no rotation policy catches, because nobody knows it happened.
//
// Every secret is given a distinctive sentinel value so a hit is unambiguous.
func TestReadinessReportOmitsSecrets(t *testing.T) {
	const (
		secretKey     = "SENTINEL-SECRET-KEY-must-never-be-logged"
		adminPassword = "SENTINEL-ADMIN-PASSWORD-must-never-be-logged"
		dsnPassword   = "SENTINEL-DSN-PASSWORD-must-never-be-logged"
	)
	cfg := loadReadinessConfig(t, map[string]string{
		"OCTARQ_SECRET_KEY":     secretKey,
		"OCTARQ_ADMIN_PASSWORD": adminPassword,
	})
	// Postgres is where a DSN carries a password. Set it on the loaded config
	// rather than through the driver env so the test needs no live database.
	cfg.DBDriver = "postgres"
	cfg.DBDSN = "postgres://octarq:" + dsnPassword + "@db.internal:5432/octarq"

	out := reportText(readinessReport(cfg, true, true))
	if out == "" {
		t.Fatal("readinessReport produced no output; the leak assertion below would be vacuous")
	}
	for name, secret := range map[string]string{
		"OCTARQ_SECRET_KEY":     secretKey,
		"OCTARQ_ADMIN_PASSWORD": adminPassword,
		"database DSN password": dsnPassword,
	} {
		if strings.Contains(out, secret) {
			t.Errorf("readiness report leaks the %s value:\n%s", name, out)
		}
	}
	// The DSN is withheld, but the operator still learns the driver.
	if !strings.Contains(out, "postgres") {
		t.Errorf("report should still name the database driver:\n%s", out)
	}
}

// TestReadinessReportFlagsSilentFailures pins the two capabilities that fail
// with no error of their own: absolute links (now: no registered domain to
// validate the request host against) and outbound mail. Each degraded line must
// also carry the action that fixes it — a status with no remedy leaves the
// operator exactly where they started.
func TestReadinessReportFlagsSilentFailures(t *testing.T) {
	cfg := loadReadinessConfig(t, nil) // sqlite, no redis: development mode

	degraded := map[string]readinessLine{}
	statuses := map[string]readinessStatus{}
	for _, l := range readinessReport(cfg, false, false) {
		statuses[l.Subject] = l.Status
		if l.Status == readyDegraded {
			degraded[l.Subject] = l
		}
	}
	for _, subject := range []string{"public origin", "outbound mail"} {
		l, ok := degraded[subject]
		if !ok {
			t.Fatalf("%q reported as %q, want degraded — this is the case that boots silently broken", subject, statuses[subject])
		}
		if !strings.Contains(l.Detail, "domain") && !strings.Contains(l.Detail, "mail.send") {
			t.Errorf("%q degraded line gives no actionable next step: %s", subject, l.Detail)
		}
	}
	if _, ok := statuses["database"]; !ok {
		t.Error("report omits the database line")
	}
	if statuses["hardening"] != readyDev {
		t.Errorf("development-mode leniency not surfaced: hardening = %q", statuses["hardening"])
	}
}

// TestReadinessReportHealthyInstance is the negative control: a fully
// configured production instance must not be told anything is degraded, or the
// degraded lines above would mean nothing.
func TestReadinessReportHealthyInstance(t *testing.T) {
	cfg := loadReadinessConfig(t, map[string]string{
		"OCTARQ_DB_DRIVER": "postgres",
		"OCTARQ_DB_DSN":    "postgres://octarq@db.internal:5432/octarq",
	})
	for _, l := range readinessReport(cfg, true, true) {
		if l.Status != readyOK {
			t.Errorf("healthy instance reported %q for %q: %s", l.Status, l.Subject, l.Detail)
		}
	}
}
