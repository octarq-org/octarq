// Package readiness evaluates an octarq instance's capabilities into one
// structured report. Two renderers consume it — the startup log (app package)
// and the instance-admin readiness API (internal/api) — but the judgment
// itself lives here, once: deriving, never duplicating.
package readiness

import (
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/config"
)

// Status is the state of one capability check.
type Status string

const (
	// StatusOK means the capability works.
	StatusOK Status = "ok"
	// StatusDegraded means the capability is limited but not broken.
	StatusDegraded Status = "degraded"
	// StatusBlocked means a capability is broken in a way that locks users out.
	StatusBlocked Status = "blocked"
	// StatusDev is the log-side leniency status: informational, fine in
	// development. The API has no dev vocabulary and collapses it to ok.
	StatusDev Status = "dev"
)

// Check is one structured readiness row. The API serializes it as-is; the log
// renderer drops ID and FixPath.
type Check struct {
	ID      string `json:"id"`
	Status  Status `json:"status"`
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	FixPath string `json:"fixPath"`
}

// Evaluate describes what this instance can and cannot do. It exists because
// several capabilities fail *silently*: an instance with no mail plugin boots
// cleanly, serves every page, and only reveals that account recovery was never
// possible when a locked-out user asks for a reset link that never arrives.
//
// mailReady must come from a real lookup of the "mail.ready" service, not from
// configuration: whether mail works depends on which plugins the composition
// root mounted AND whether any SMTP sender is configured. requireEmailVerification
// feeds the self-contradiction check: an instance that demands verified emails
// but cannot send them has a broken registration flow — the exact dead end the
// verification gate is meant to prevent.
//
// The result NEVER includes a secret value. The secret key, the admin password
// and any DSN password are reported as configured / not configured only — this
// output reaches the operator's log aggregator and the instance-admin API,
// neither of which is a place for the KEK.
func Evaluate(cfg *config.Config, mailReady, domainsRegistered, requireEmailVerification bool) []Check {
	checks := []Check{
		// Absolute URLs come from the request host, checked against the
		// registered domains (origin). With none registered there is nothing to
		// check against and the request host is used as sent — worth saying out
		// loud, because it is the one state in which a forged Host header could
		// aim a password-reset link somewhere else.
		{
			ID: "public-origin", Title: "public origin", FixPath: "/domains",
			Status: boolStatus(domainsRegistered),
			Detail: boolDetail(domainsRegistered,
				"password-reset, verification and invite links are built from the request host, accepted only when it matches a registered domain",
				"no domain is registered, so links are built from the request host as sent, with nothing to validate it against. Add the domain this instance is served on (Domains → Add domain) to have octarq reject hostnames it does not own"),
		},
		{
			ID: "outbound-mail", Title: "outbound mail", FixPath: "/mail?tab=settings",
			Status: boolStatus(mailReady),
			Detail: boolDetail(mailReady,
				"the system sender is available; password reset, email verification and invites can be delivered",
				"no SMTP sender is configured, so the system sender is unavailable — password-reset, verification and invite messages will NOT be delivered and a locked-out user cannot recover their account. Configure an SMTP sender (Mail → SMTP senders), or mount a plugin providing mail.send"),
		},
		{
			ID: "registration", Title: "registration", FixPath: "/mail?tab=settings",
			Status: registrationStatus(requireEmailVerification, mailReady),
			Detail: registrationDetail(requireEmailVerification, mailReady),
		},
		{
			ID: "database", Title: "database",
			Status: StatusOK,
			Detail: fmt.Sprintf("driver=%s dsn=%s", cfg.DBDriver, redactDSN(cfg.DBDriver, cfg.DBDSN)),
		},
	}

	if !cfg.Provisioned() {
		checks = append(checks, Check{
			ID: "hardening", Title: "hardening", Status: StatusDev,
			Detail: "running on the default sqlite file with no Redis, which is treated as development: the secret-key length floor warns instead of refusing to start only when no domain is registered. Registering any domain enforces it",
		})
	}
	if len(cfg.SecretKey) < config.MinSecretKeyLen {
		if domainsRegistered {
			checks = append(checks, Check{
				ID: "secret-key", Title: "secret key", Status: StatusDegraded,
				Detail: fmt.Sprintf("configured but shorter than %d bytes when a domain is registered: instance will refuse to boot. Set OCTARQ_SECRET_KEY to at least %d bytes (`openssl rand -hex 32`)", config.MinSecretKeyLen, config.MinSecretKeyLen),
			})
		} else {
			checks = append(checks, Check{
				ID: "secret-key", Title: "secret key", Status: StatusDev,
				Detail: fmt.Sprintf("configured but shorter than %d bytes, which is accepted only in development. Set OCTARQ_SECRET_KEY to at least %d bytes (`openssl rand -hex 32`) before production", config.MinSecretKeyLen, config.MinSecretKeyLen),
			})
		}
	} else {
		checks = append(checks, Check{
			ID: "secret-key", Title: "secret key", Status: StatusOK,
			Detail: "configured",
		})
	}
	return checks
}

// registrationStatus is blocked when the instance demands verified emails but
// the system sender cannot deliver them — the sign-up dead end the verification
// gate exists to prevent.
func registrationStatus(requireEmailVerification, mailReady bool) Status {
	if requireEmailVerification && !mailReady {
		return StatusBlocked
	}
	return StatusOK
}

func registrationDetail(requireEmailVerification, mailReady bool) string {
	if requireEmailVerification && !mailReady {
		return "registration is currently broken: new users will be stuck at the verification-email step, because the instance requires a verified email but no SMTP sender is configured. Configure a system sender (Mail → SMTP senders), or disable email verification (Instance settings)"
	}
	if !requireEmailVerification {
		return "email verification is disabled, so sign-up does not depend on the system sender"
	}
	return "email verification is enabled and the system sender is available; new users can complete sign-up"
}

func boolStatus(ok bool) Status {
	if ok {
		return StatusOK
	}
	return StatusDegraded
}

func boolDetail(ok bool, whenOK, whenNot string) string {
	if ok {
		return whenOK
	}
	return whenNot
}

// redactDSN renders a data source name safely. A sqlite DSN is a file path and
// is the operator's most useful "where is my data" signal, so it is shown;
// every other driver's DSN embeds a password (postgres://user:pass@host/db),
// and no partial-masking scheme survives the shapes those strings take —
// key/value pairs, URL userinfo, appended options — so the whole thing is
// withheld.
func redactDSN(driver, dsn string) string {
	if driver == "sqlite" {
		return dsn
	}
	if strings.TrimSpace(dsn) == "" {
		return "(not configured)"
	}
	return "(configured, withheld: may contain a password)"
}
