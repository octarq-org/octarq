package llmgateway

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestPricingServiceWithoutAStoreIsUnavailable(t *testing.T) {
	if _, err := (pricingService{}).Price(context.Background(), 1, "m"); !errors.Is(err, plugin.ErrPricingUnavailable) {
		t.Fatalf("nil plugin = %v, want ErrPricingUnavailable", err)
	}
	if _, err := (pricingService{plugin: &Plugin{}}).Price(context.Background(), 1, "m"); !errors.Is(err, plugin.ErrPricingUnavailable) {
		t.Fatalf("nil store = %v, want ErrPricingUnavailable", err)
	}
}
