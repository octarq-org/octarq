package llmgateway

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/server/plugin"
)

//go:embed prices.json
var pricesJSON []byte

type priceEntry struct {
	ModelID     string `json:"model_id"`
	DisplayName string `json:"display_name"`
	ProviderID  string `json:"provider_id"`
	Pricing     struct {
		Input       *float64 `json:"input"`
		Output      *float64 `json:"output"`
		CachedInput *float64 `json:"cached_input"`
		Unit        string   `json:"unit"`
		Currency    string   `json:"currency"`
	} `json:"pricing"`
}

type priceCatalog struct {
	Source         string `json:"source"`
	DatasetName    string `json:"dataset_name"`
	DatasetVersion string `json:"dataset_version"`
	GeneratedAt    string `json:"generated_at"`
	Licenses       struct {
		Code string `json:"code"`
		Data string `json:"data"`
	} `json:"licenses"`
	ModelCount int          `json:"model_count"`
	Models     []priceEntry `json:"models"`

	byID      map[string]int
	bySuffix  map[string]int
	ambiguous map[string]bool
}

var (
	catalogOnce sync.Once
	catalogData *priceCatalog
	catalogErr  error
)

func pricingCatalog() (*priceCatalog, error) {
	catalogOnce.Do(func() {
		catalogData, catalogErr = parsePriceCatalog(pricesJSON)
	})
	return catalogData, catalogErr
}

func parsePriceCatalog(raw []byte) (*priceCatalog, error) {
	var c priceCatalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("llm gateway: parse price catalog: %w", err)
	}
	if len(c.Models) == 0 {
		return nil, fmt.Errorf("llm gateway: price catalog holds no models")
	}
	if c.ModelCount != len(c.Models) {
		return nil, fmt.Errorf("llm gateway: price catalog declares %d models but holds %d", c.ModelCount, len(c.Models))
	}
	c.byID = make(map[string]int, len(c.Models))
	c.bySuffix = make(map[string]int, len(c.Models))
	c.ambiguous = map[string]bool{}
	for i, e := range c.Models {
		id := strings.ToLower(strings.TrimSpace(e.ModelID))
		if id == "" {
			return nil, fmt.Errorf("llm gateway: price catalog row %d has no model_id", i)
		}
		if _, dup := c.byID[id]; dup {
			return nil, fmt.Errorf("llm gateway: price catalog repeats model_id %q", e.ModelID)
		}
		if unit := strings.ToLower(strings.TrimSpace(e.Pricing.Unit)); unit != "" && unit != "1m tokens" {
			return nil, fmt.Errorf("llm gateway: price catalog row %d (%s) has unit %q, want 1M tokens", i, e.ModelID, e.Pricing.Unit)
		}
		if currency := strings.ToUpper(strings.TrimSpace(e.Pricing.Currency)); currency != "" && currency != "USD" {
			return nil, fmt.Errorf("llm gateway: price catalog row %d (%s) has currency %q, want USD", i, e.ModelID, e.Pricing.Currency)
		}
		for _, p := range []struct {
			name  string
			value *float64
		}{
			{"input", e.Pricing.Input},
			{"output", e.Pricing.Output},
			{"cached_input", e.Pricing.CachedInput},
		} {
			if p.value != nil && *p.value < 0 {
				return nil, fmt.Errorf("llm gateway: price catalog row %d (%s) has a negative %s price", i, e.ModelID, p.name)
			}
		}
		c.byID[id] = i
		suffix := id
		if slash := strings.LastIndex(id, "/"); slash >= 0 {
			suffix = id[slash+1:]
		}
		if suffix == "" || c.ambiguous[suffix] {
			continue
		}
		if _, taken := c.bySuffix[suffix]; taken {
			c.ambiguous[suffix] = true
			delete(c.bySuffix, suffix)
			continue
		}
		c.bySuffix[suffix] = i
	}
	return &c, nil
}

func (c *priceCatalog) price(model string) (plugin.ModelPrice, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return plugin.ModelPrice{}, plugin.ErrModelUnpriced
	}
	key := strings.ToLower(model)
	idx, ok := c.byID[key]
	if !ok {
		if slash := strings.LastIndex(key, "/"); slash >= 0 && slash < len(key)-1 {
			suffix := key[slash+1:]
			if !c.ambiguous[suffix] {
				idx, ok = c.bySuffix[suffix]
			}
		}
	}
	if !ok {
		return plugin.ModelPrice{}, plugin.ErrModelUnpriced
	}
	e := c.Models[idx]
	if e.Pricing.Input == nil || e.Pricing.Output == nil {
		return plugin.ModelPrice{}, plugin.ErrModelUnpriced
	}

	return plugin.ModelPrice{Input: *e.Pricing.Input, Output: *e.Pricing.Output, Reasoning: *e.Pricing.Output}, nil
}

func (c *priceCatalog) billable() []PriceDTO {
	out := make([]PriceDTO, 0, len(c.Models))
	for _, e := range c.Models {
		if e.Pricing.Input == nil || e.Pricing.Output == nil {
			continue
		}
		out = append(out, PriceDTO{
			Model:       e.ModelID,
			DisplayName: e.DisplayName,
			Input:       *e.Pricing.Input,
			Output:      *e.Pricing.Output,
			Reasoning:   *e.Pricing.Output,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}
