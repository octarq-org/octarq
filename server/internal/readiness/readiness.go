package readiness

import (
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/config"
)

type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
	StatusBlocked  Status = "blocked"
	StatusDev      Status = "dev"
)

type Check struct {
	ID      string `json:"id"`
	Status  Status `json:"status"`
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	FixPath string `json:"fixPath"`
}

func Evaluate(cfg *config.Config, mailReady, domainsRegistered, requireEmailVerification bool) []Check {
	checks := []Check{
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
				"no SMTP sender is configured, so the system sender is unavailable. Password-reset, verification and invite messages will not be delivered, and a locked-out user cannot recover their account. Configure an SMTP sender (Mail → SMTP senders) or mount a plugin providing mail.send"),
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

func redactDSN(driver, dsn string) string {
	if driver == "sqlite" {
		return dsn
	}
	if strings.TrimSpace(dsn) == "" {
		return "(not configured)"
	}
	return "(configured, withheld: may contain a password)"
}
