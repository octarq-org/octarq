package plugin

import (
	"context"
	"errors"

	"github.com/octarq-org/octarq/server/llmprovider"
)

// ServiceLLMProvider is where the LLM gateway registers its per-workspace
// backend factory.
const ServiceLLMProvider = "llmgateway.provider"

// LLMGateway builds the LLM backend a workspace configured, with that
// workspace's own credentials.
//
// A feature that calls a model looks this up instead of keeping its own
// provider table: one registry means the price a budget is enforced against and
// the provider the turn runs on come from the same row.
type LLMGateway interface {
	// Provider returns the workspace's active backend. It fails when the
	// workspace configured none, or when the gateway cannot decrypt the key.
	Provider(ctx context.Context, orgID uint) (llmprovider.Provider, error)
}

// ErrLLMGatewayUnavailable is returned when no gateway is mounted.
var ErrLLMGatewayUnavailable = errors.New("llm gateway: no gateway is mounted")
