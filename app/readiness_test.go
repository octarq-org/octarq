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

	out := reportText(readinessReport(cfg, true, true, true))
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
	for _, l := range readinessReport(cfg, false, false, true) {
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
	for _, l := range readinessReport(cfg, true, true, true) {
		if l.Status != readyOK {
			t.Errorf("healthy instance reported %q for %q: %s", l.Status, l.Subject, l.Detail)
		}
	}
}

func TestEnforceSecretKeyFloor(t *testing.T) {
	shortKey := "s3cr3t"
	longKey := "0123456789abcdef0123456789abcdef"

	t.Run("four quadrants", func(t *testing.T) {
		tests := []struct {
			name              string
			key               string
			domainsRegistered bool
			wantErr           bool
		}{
			{
				name:              "registered and short key",
				key:               shortKey,
				domainsRegistered: true,
				wantErr:           true,
			},
			{
				name:              "registered and long key",
				key:               longKey,
				domainsRegistered: true,
				wantErr:           false,
			},
			{
				name:              "unregistered and short key",
				key:               shortKey,
				domainsRegistered: false,
				wantErr:           false,
			},
			{
				name:              "unregistered and long key",
				key:               longKey,
				domainsRegistered: false,
				wantErr:           false,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				cfg := loadReadinessConfig(t, map[string]string{
					"OCTARQ_SECRET_KEY": tc.key,
				})
				err := enforceSecretKeyFloor(cfg, tc.domainsRegistered)
				if (err != nil) != tc.wantErr {
					t.Errorf("enforceSecretKeyFloor() error = %v, wantErr %v", err, tc.wantErr)
				}
			})
		}
	})

	t.Run("error message omits secret key value", func(t *testing.T) {
		cfg := loadReadinessConfig(t, map[string]string{
			"OCTARQ_SECRET_KEY": shortKey,
		})
		err := enforceSecretKeyFloor(cfg, true)
		if err == nil {
			t.Fatal("expected error when domains registered with short key, got nil")
		}
		if strings.Contains(err.Error(), shortKey) {
			t.Errorf("enforceSecretKeyFloor error leaks secret key value: %v", err)
		}
	})

	t.Run("readiness report secret key statuses", func(t *testing.T) {
		// 1) Registered + short key -> readyDegraded
		cfgShort := loadReadinessConfig(t, map[string]string{
			"OCTARQ_SECRET_KEY": shortKey,
		})
		linesRegShort := readinessReport(cfgShort, true, true, true)
		var statusRegShort readinessStatus
		for _, l := range linesRegShort {
			if l.Subject == "secret key" {
				statusRegShort = l.Status
			}
		}
		if statusRegShort != readyDegraded {
			t.Errorf("secret key status for registered domain with short key = %q, want %q", statusRegShort, readyDegraded)
		}

		// 2) Unregistered + short key -> readyDev
		linesUnregShort := readinessReport(cfgShort, true, false, true)
		var statusUnregShort readinessStatus
		for _, l := range linesUnregShort {
			if l.Subject == "secret key" {
				statusUnregShort = l.Status
			}
		}
		if statusUnregShort != readyDev {
			t.Errorf("secret key status for unregistered domain with short key = %q, want %q", statusUnregShort, readyDev)
		}

		// 3) Long key -> readyOK
		cfgLong := loadReadinessConfig(t, map[string]string{
			"OCTARQ_SECRET_KEY": longKey,
		})
		linesLong := readinessReport(cfgLong, true, true, true)
		var statusLong readinessStatus
		for _, l := range linesLong {
			if l.Subject == "secret key" {
				statusLong = l.Status
			}
		}
		if statusLong != readyOK {
			t.Errorf("secret key status for long key = %q, want %q", statusLong, readyOK)
		}
	})
}
