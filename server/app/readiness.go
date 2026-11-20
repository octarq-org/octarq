package app

import (
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/readiness"
)

const (
	readyOK       = readiness.StatusOK
	readyDegraded = readiness.StatusDegraded
	readyDev      = readiness.StatusDev
	readyBlocked  = readiness.StatusBlocked
)

type readinessStatus = readiness.Status

type readinessLine struct {
	Status  readiness.Status
	Subject string
	Detail  string
}

func (l readinessLine) String() string {
	return fmt.Sprintf("[%s] %s: %s", strings.ToUpper(string(l.Status)), l.Subject, l.Detail)
}

func readinessReport(cfg *config.Config, mailReady, domainsRegistered, requireEmailVerification bool) []readinessLine {
	var lines []readinessLine
	for _, c := range readiness.Evaluate(cfg, mailReady, domainsRegistered, requireEmailVerification) {
		lines = append(lines, readinessLine{Status: c.Status, Subject: c.Title, Detail: c.Detail})
	}
	return lines
}

func enforceSecretKeyFloor(cfg *config.Config, domainsRegistered bool) error {
	if domainsRegistered && len(cfg.SecretKey) < config.MinSecretKeyLen {
		return fmt.Errorf("OCTARQ_SECRET_KEY must be at least %d bytes when a domain is registered; set OCTARQ_SECRET_KEY to a longer value (e.g. `openssl rand -hex 32`). WARNING: OCTARQ_SECRET_KEY is also the primary key for credential encryption; changing it will render existing encrypted credentials (such as TOTP keys and plugin credentials) un-decryptable", config.MinSecretKeyLen)
	}
	return nil
}
