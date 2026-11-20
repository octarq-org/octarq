package plugin

import (
	"context"
	"errors"
)

type ModelPrice struct {
	Input     float64 `json:"input"`
	Output    float64 `json:"output"`
	Reasoning float64 `json:"reasoning"`
}

type ModelPricing interface {
	Price(ctx context.Context, orgID uint, model string) (ModelPrice, error)
}

const ServiceModelPricing = "llmgateway.pricing"

var ErrModelUnpriced = errors.New("pricing: the pricing catalog has no price for the model")

var ErrPricingUnavailable = errors.New("pricing: no price registry is mounted")
