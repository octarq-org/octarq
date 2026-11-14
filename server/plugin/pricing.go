package plugin

import (
	"context"
	"errors"
)

// ModelPrice is the price of one model per one million tokens, in USD.
type ModelPrice struct {
	Input     float64 `json:"input"`
	Output    float64 `json:"output"`
	Reasoning float64 `json:"reasoning"`
}

// ModelPricing is the host's model price registry. A billing ledger asks it
// what a turn costs, so the price a ceiling is enforced against and the price
// the ledger charges are the same number.
//
// The LLM gateway plugin provides this under ServiceModelPricing. A build
// without the gateway has no prices at all, and every call must fail rather
// than bill at zero.
type ModelPricing interface {
	Price(ctx context.Context, orgID uint, model string) (ModelPrice, error)
}

// ServiceModelPricing is the service name the LLM gateway registers.
const ServiceModelPricing = "llmgateway.pricing"

// ErrModelUnpriced is returned when the workspace has no price for the model.
// A model without a price is refused instead of billed at zero, because a cost
// ceiling over a zero price could never trip.
var ErrModelUnpriced = errors.New("pricing: the model has no price in this workspace")

// ErrPricingUnavailable is returned when no price registry is mounted.
var ErrPricingUnavailable = errors.New("pricing: no price registry is mounted")
