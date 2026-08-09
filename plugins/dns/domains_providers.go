package dns

import (
	"context"
	"log"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/dnsprovider"
)

func cleanProviderError(errStr string) string {
	lower := strings.ToLower(errStr)
	switch {
	case strings.Contains(lower, "token"), strings.Contains(lower, "auth"), strings.Contains(lower, "credential"), strings.Contains(lower, "permission"), strings.Contains(lower, "unauthorized"), strings.Contains(lower, "401"), strings.Contains(lower, "403"):
		return "authentication failed — check your API token or credentials"
	// Absence is matched before existence: "does not exist" contains "exist",
	// so a bare "exist" test would report a missing record as a duplicate.
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

// providerErr logs an upstream DNS-provider failure and returns it as a 400 so
// the real cause is logged, but third-party response bodies are sanitized.
func (p *Plugin) providerErr(action string, err error) error {
	log.Printf("dns provider: %s failed: %v", action, err)
	if err == nil {
		return huma.Error400BadRequest(action + ": failed")
	}
	return huma.Error400BadRequest(action + ": " + cleanProviderError(err.Error()))
}

type DNSProvidersInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *DNSProvidersInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DNSProvidersOutput struct {
	Body []string
}

func (p *Plugin) dnsProviders(ctx context.Context, input *DNSProvidersInput) (*DNSProvidersOutput, error) {
	return &DNSProvidersOutput{Body: dnsprovider.Names()}, nil
}
