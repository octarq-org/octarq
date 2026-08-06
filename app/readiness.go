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
// instance with no mail plugin boots cleanly, serves every page, and only
// reveals that account recovery was never possible when a locked-out user asks
// for a reset link that never arrives.
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
func readinessReport(cfg *config.Config, mailAvailable, domainsRegistered bool) []readinessLine {
	var lines []readinessLine

	// Absolute URLs come from the request host, checked against the registered
	// domains (origin). With none registered there is nothing to check
	// against and the request host is used as sent — worth saying out loud,
	// because it is the one state in which a forged Host header could aim a
	// password-reset link somewhere else.
	if domainsRegistered {
		lines = append(lines, readinessLine{readyOK, "public origin",
			"password-reset, verification and invite links are built from the request host, accepted only when it matches a registered domain"})
	} else {
		lines = append(lines, readinessLine{readyDegraded, "public origin",
			"no domain is registered, so links are built from the request host as sent, with nothing to validate it against. Add the domain this instance is served on (Domains → Add domain) to have octarq reject hostnames it does not own"})
	}

	if mailAvailable {
		lines = append(lines, readinessLine{readyOK, "outbound mail", "a mail plugin provides mail.send; password reset and email verification can be delivered"})
	} else {
		lines = append(lines, readinessLine{readyDegraded, "outbound mail",
			"no plugin provides mail.send — password-reset and email-verification messages will NOT be delivered and a locked-out user cannot recover their account. Mount the mail plugin (plugins/builtin) or another plugin providing mail.send"})
	}

	lines = append(lines, readinessLine{readyOK, "database", fmt.Sprintf("driver=%s dsn=%s", cfg.DBDriver, redactDSN(cfg.DBDriver, cfg.DBDSN))})

	if !cfg.Provisioned() {
		lines = append(lines, readinessLine{readyDev, "hardening",
			"running on the default sqlite file with no Redis, which is treated as development: the secret-key length floor warns instead of refusing to start only when no domain is registered. Registering any domain enforces it"})
	}
	if len(cfg.SecretKey) < config.MinSecretKeyLen {
		if domainsRegistered {
			lines = append(lines, readinessLine{readyDegraded, "secret key",
				fmt.Sprintf("configured but shorter than %d bytes when a domain is registered: instance will refuse to boot. Set OCTARQ_SECRET_KEY to at least %d bytes (`openssl rand -hex 32`)", config.MinSecretKeyLen, config.MinSecretKeyLen)})
		} else {
			lines = append(lines, readinessLine{readyDev, "secret key",
				fmt.Sprintf("configured but shorter than %d bytes, which is accepted only in development. Set OCTARQ_SECRET_KEY to at least %d bytes (`openssl rand -hex 32`) before production", config.MinSecretKeyLen, config.MinSecretKeyLen)})
		}
	} else {
		lines = append(lines, readinessLine{readyOK, "secret key", "configured"})
	}

	return lines
}

// enforceSecretKeyFloor returns an error when this instance must not boot
// because a registered domain indicates a production deployment with a secret key
// shorter than config.MinSecretKeyLen.
func enforceSecretKeyFloor(cfg *config.Config, domainsRegistered bool) error {
	if domainsRegistered && len(cfg.SecretKey) < config.MinSecretKeyLen {
		return fmt.Errorf("OCTARQ_SECRET_KEY must be at least %d bytes when a domain is registered; set OCTARQ_SECRET_KEY to a longer value (e.g. `openssl rand -hex 32`). WARNING: OCTARQ_SECRET_KEY is also the primary key for credential encryption; changing it will render existing encrypted credentials (such as TOTP keys and plugin credentials) un-decryptable", config.MinSecretKeyLen)
	}
	return nil
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
