package app

import (
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/config"
)

// readinessStatus labels one line of the startup readiness report.
type readinessStatus string

const (
	readyOK       readinessStatus = "ok"
	readyDegraded readinessStatus = "degraded"
	readyDev      readinessStatus = "dev"
)

// readinessLine is one capability row: what was checked, whether it works, and
// — when it does not — the single command-line change that fixes it. A status
// with no remedy is a dead end for the operator, so degraded rows always carry
// one.
type readinessLine struct {
	Status  readinessStatus
	Subject string
	Detail  string
}

func (l readinessLine) String() string {
	return fmt.Sprintf("[%s] %s: %s", strings.ToUpper(string(l.Status)), l.Subject, l.Detail)
}

// readinessReport describes what this instance can and cannot do, for printing
// once at startup. It exists because several capabilities fail *silently*: an
// instance with no BaseURL and no mail plugin boots cleanly, serves every page,
// and only reveals that account recovery was never possible when a locked-out
// user asks for a reset link that never arrives.
//
// mailAvailable must come from a real lookup of the "mail.send" service, not
// from configuration: whether mail works depends on which plugins the
// composition root mounted, which is why Run calls this after the mount loop
// rather than at New() time.
//
// It NEVER includes a secret value. The secret key, the admin password and any
// DSN password are reported as configured / not configured only — this output
// goes to the operator's log aggregator, which is not a place to put the KEK.
// TestReadinessReportOmitsSecrets pins that.
func readinessReport(cfg *config.Config, mailAvailable bool) []readinessLine {
	var lines []readinessLine

	if strings.TrimSpace(cfg.BaseURL) == "" {
		lines = append(lines, readinessLine{readyDegraded, "public base URL",
			"not configured — password-reset and email-verification links, OAuth callbacks and webhook addresses will be relative paths and cannot be opened. Set OCTARQ_BASE_URL=https://your.domain"})
	} else {
		lines = append(lines, readinessLine{readyOK, "public base URL", cfg.BaseURL})
	}

	if mailAvailable {
		lines = append(lines, readinessLine{readyOK, "outbound mail", "a mail plugin provides mail.send; password reset and email verification can be delivered"})
	} else {
		lines = append(lines, readinessLine{readyDegraded, "outbound mail",
			"no plugin provides mail.send — password-reset and email-verification messages will NOT be delivered and a locked-out user cannot recover their account. Mount the mail plugin (plugins/builtin) or another plugin providing mail.send"})
	}

	lines = append(lines, readinessLine{readyOK, "database", fmt.Sprintf("driver=%s dsn=%s", cfg.DBDriver, redactDSN(cfg.DBDriver, cfg.DBDSN))})

	if !cfg.IsProduction() {
		lines = append(lines, readinessLine{readyDev, "hardening",
			"development mode — session cookies are not marked Secure and production-only checks are relaxed. Set OCTARQ_SECURE_COOKIES=true (or serve an https OCTARQ_BASE_URL) before exposing this instance"})
	}
	if len(cfg.SecretKey) < config.MinSecretKeyLen {
		lines = append(lines, readinessLine{readyDev, "secret key",
			fmt.Sprintf("configured but shorter than %d bytes, which is accepted only in development. Set OCTARQ_SECRET_KEY to at least %d bytes (`openssl rand -hex 32`) before production", config.MinSecretKeyLen, config.MinSecretKeyLen)})
	} else {
		lines = append(lines, readinessLine{readyOK, "secret key", "configured"})
	}

	return lines
}

// redactDSN renders a data source name safely. A sqlite DSN is a file path and
// is the operator's most useful "where is my data" signal, so it is shown; every
// other driver's DSN embeds a password (postgres://user:pass@host/db), and no
// partial-masking scheme survives the shapes those strings take — key/value
// pairs, URL userinfo, appended options — so the whole thing is withheld.
func redactDSN(driver, dsn string) string {
	if driver == "sqlite" {
		return dsn
	}
	if strings.TrimSpace(dsn) == "" {
		return "(not configured)"
	}
	return "(configured, withheld: may contain a password)"
}
