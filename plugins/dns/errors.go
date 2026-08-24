package dns

import (
	"log"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/origin"
)

func forgetOrigin(names ...string) {
	for _, n := range names {
		origin.ClearDomainCache(n)
	}
}

func isAuthError(lower string) bool {
	for _, term := range []string{"token", "auth", "credential", "permission", "unauthorized", "401", "403"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func cleanProviderError(errStr string) string {
	lower := strings.ToLower(errStr)
	switch {
	case isAuthError(lower):
		return "authentication failed — check your API token or credentials"
	case strings.Contains(lower, "not found"), strings.Contains(lower, "not exist"), strings.Contains(lower, "nxdomain"), strings.Contains(lower, "missing"):
		return "record or zone not found"
	case strings.Contains(lower, "already exists"), strings.Contains(lower, "duplicate"):
		return "record already exists"
	case strings.Contains(lower, "invalid"), strings.Contains(lower, "validation"), strings.Contains(lower, "parameter"):
		return "invalid record configuration or parameters"
	default:
		return "provider request failed"
	}
}

func (p *Plugin) providerErr(action string, err error) error {
	log.Printf("dns provider: %s failed: %v", action, err)
	if err == nil {
		return huma.Error400BadRequest(action + ": failed")
	}
	return huma.Error400BadRequest(action + ": " + cleanProviderError(err.Error()))
}
