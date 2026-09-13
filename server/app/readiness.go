package app

import (
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/readiness"
)

// Log-side status vocabulary. These alias the shared readiness statuses so the
// startup log keeps its four-value vocabulary (dev is log-only); the readiness
// API collapses dev into ok.
const (
	readyOK       = readiness.StatusOK
	readyDegraded = readiness.StatusDegraded
	readyDev      = readiness.StatusDev
	readyBlocked  = readiness.StatusBlocked
)

// readinessStatus is the historical name of the status type, kept as an alias
// for the log renderer and its tests.
type readinessStatus = readiness.Status

// readinessLine is one log row: what was checked, whether it works, and — when
// it does not — the single command-line change that fixes it. A status with no
// remedy is a dead end for the operator, so degraded rows always carry one.
// The judgment itself lives in readiness.Evaluate; this type only renders it
// for the log.
type readinessLine struct {
	Status  readiness.Status
	Subject string
	Detail  string
}

func (l readinessLine) String() string {
	return fmt.Sprintf("[%s] %s: %s", strings.ToUpper(string(l.Status)), l.Subject, l.Detail)
}

// readinessReport renders readiness.Evaluate — the single source of truth for
// capability judgment — as the startup log's line form. It exists because
// several capabilities fail *silently*: an instance with no mail plugin boots
// cleanly, serves every page, and only reveals that account recovery was never
// possible when a locked-out user asks for a reset link that never arrives.
//
// mailReady must come from a real lookup of the "mail.ready" service, not from
// configuration: whether mail works depends on which plugins the composition
// root mounted AND whether any SMTP sender is configured, which is why Run
// calls this after the mount loop rather than at New() time.
//
// It NEVER includes a secret value — readiness.Evaluate only ever reports
// configured / not configured. TestReadinessReportOmitsSecrets pins that.
func readinessReport(cfg *config.Config, mailReady, domainsRegistered, requireEmailVerification bool) []readinessLine {
	var lines []readinessLine
	for _, c := range readiness.Evaluate(cfg, mailReady, domainsRegistered, requireEmailVerification) {
		lines = append(lines, readinessLine{Status: c.Status, Subject: c.Title, Detail: c.Detail})
	}
	return lines
}

// enforceSecretKeyFloor is the second half of the strictness predicate whose
// first half is config.Provisioned — see the comment there.
//
// A registered domain means the operator has attached this instance to a
// hostname they own, i.e. it is served to the public. That is the signal that
// replaced the https OCTARQ_BASE_URL the floor used to key on, and it can only
// be read here, after the database is open. Without it a sqlite instance behind
// a TLS proxy — the most ordinary self-hosted shape there is — would boot with a
// guessable key protecting both session cookies and every stored credential.
//
// Refusing to boot, rather than warning, is the point: a warning in a log
// aggregator is a warning nobody reads.
func enforceSecretKeyFloor(cfg *config.Config, domainsRegistered bool) error {
	if domainsRegistered && len(cfg.SecretKey) < config.MinSecretKeyLen {
		return fmt.Errorf("OCTARQ_SECRET_KEY must be at least %d bytes when a domain is registered; set OCTARQ_SECRET_KEY to a longer value (e.g. `openssl rand -hex 32`). WARNING: OCTARQ_SECRET_KEY is also the primary key for credential encryption; changing it will render existing encrypted credentials (such as TOTP keys and plugin credentials) un-decryptable", config.MinSecretKeyLen)
	}
	return nil
}
