package plugin

import (
	"context"
	"errors"

	"github.com/octarq-org/octarq/server/llmprovider"
)

const ServiceLLMProvider = "llmgateway.provider"

type LLMGateway interface {
	Provider(ctx context.Context, orgID uint) (llmprovider.Provider, error)
}

var ErrLLMGatewayUnavailable = errors.New("llm gateway: no gateway is mounted")
