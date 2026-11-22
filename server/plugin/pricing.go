package plugin

import (
	"context"
	"errors"
)

type ModelPrice struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	Reasoning  float64 `json:"reasoning"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

type ModelPricing interface {
	Price(ctx context.Context, orgID uint, model string) (ModelPrice, error)
}

const ServiceModelPricing = "llmgateway.pricing"

var ErrModelUnpriced = errors.New("pricing: the pricing catalog has no price for the model")

var ErrModelAmbiguous = errors.New("pricing: several providers publish this model at different prices")

var ErrPricingUnavailable = errors.New("pricing: no price registry is mounted")
