package llmgateway

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/server/plugin"
)

//go:embed bundled/models.json.gz
var bundledModelsGz []byte

var snapshotRegex = regexp.MustCompile(`(-20\d{6}|-20\d{2}-\d{2}-\d{2})$`)

type bundleRow struct {
	modelID    string
	providerID string
	input      float64
	output     float64
	cacheRead  float64
	cacheWrite float64
	reasoning  float64
	name       string
}

func (r bundleRow) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{
		r.modelID,
		r.providerID,
		r.input,
		r.output,
		r.cacheRead,
		r.cacheWrite,
		r.reasoning,
		r.name,
	})
}

func (r *bundleRow) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) < 8 {
		return fmt.Errorf("llm gateway: row has %d elements, want 8", len(raw))
	}
	if err := json.Unmarshal(raw[0], &r.modelID); err != nil || strings.TrimSpace(r.modelID) == "" {
		return fmt.Errorf("llm gateway: invalid model_id: %s", string(raw[0]))
	}
	if err := json.Unmarshal(raw[1], &r.providerID); err != nil || strings.TrimSpace(r.providerID) == "" {
		return fmt.Errorf("llm gateway: invalid provider_id: %s", string(raw[1]))
	}
	var inputPtr *float64
	if err := json.Unmarshal(raw[2], &inputPtr); err != nil || inputPtr == nil || *inputPtr < 0 {
		return fmt.Errorf("llm gateway: invalid input price: %s", string(raw[2]))
	}
	r.input = *inputPtr

	var outputPtr *float64
	if err := json.Unmarshal(raw[3], &outputPtr); err != nil || outputPtr == nil || *outputPtr < 0 {
		return fmt.Errorf("llm gateway: invalid output price: %s", string(raw[3]))
	}
	r.output = *outputPtr

	var cacheReadPtr *float64
	if err := json.Unmarshal(raw[4], &cacheReadPtr); err == nil && cacheReadPtr != nil {
		if *cacheReadPtr < 0 {
			return fmt.Errorf("llm gateway: negative cache_read: %f", *cacheReadPtr)
		}
		r.cacheRead = *cacheReadPtr
	}
	var cacheWritePtr *float64
	if err := json.Unmarshal(raw[5], &cacheWritePtr); err == nil && cacheWritePtr != nil {
		if *cacheWritePtr < 0 {
			return fmt.Errorf("llm gateway: negative cache_write: %f", *cacheWritePtr)
		}
		r.cacheWrite = *cacheWritePtr
	}
	var reasoningPtr *float64
	if err := json.Unmarshal(raw[6], &reasoningPtr); err == nil && reasoningPtr != nil {
		if *reasoningPtr < 0 {
			return fmt.Errorf("llm gateway: negative reasoning: %f", *reasoningPtr)
		}
		r.reasoning = *reasoningPtr
	}
	if err := json.Unmarshal(raw[7], &r.name); err != nil {
		return fmt.Errorf("llm gateway: invalid name: %s", string(raw[7]))
	}
	r.name = strings.TrimSpace(r.name)
	return nil
}

type bundleFile struct {
	GeneratedAt string      `json:"generated_at"`
	Source      string      `json:"source"`
	Models      []bundleRow `json:"models"`
}

type catalogEntry struct {
	modelID    string
	providerID string
	name       string
	price      plugin.ModelPrice
}

type Catalog struct {
	mu          sync.RWMutex
	generatedAt string
	source      string
	entries     []catalogEntry
	qualified   map[string]catalogEntry
	bare        map[string][]catalogEntry
	providers   map[string]bool
}

var (
	catalogMu     sync.RWMutex
	globalCatalog *Catalog
	seedCatalog   *Catalog
	seedOnce      sync.Once
	seedErr       error
)

func initSeedCatalog() (*Catalog, error) {
	seedOnce.Do(func() {
		seedCatalog, seedErr = ParseBundleGz(bundledModelsGz)
	})
	if seedErr == nil && seedCatalog != nil {
		catalogMu.Lock()
		if globalCatalog == nil {
			globalCatalog = seedCatalog
		}
		catalogMu.Unlock()
	}
	return seedCatalog, seedErr
}

func loadCatalog() *Catalog {
	catalogMu.RLock()
	c := globalCatalog
	catalogMu.RUnlock()
	if c != nil {
		return c
	}
	seed, _ := initSeedCatalog()
	catalogMu.Lock()
	if globalCatalog == nil {
		globalCatalog = seed
	}
	c = globalCatalog
	catalogMu.Unlock()
	return c
}

func replaceCatalog(c *Catalog) {
	catalogMu.Lock()
	globalCatalog = c
	catalogMu.Unlock()
}

func displayName(e catalogEntry) string {
	if e.name != "" {
		return e.name
	}
	return e.modelID
}

func normalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ".", "-")
	return s
}

func (c *Catalog) lookupQualified(p, norm string) (catalogEntry, bool) {
	if entry, ok := c.qualified[p+"/"+norm]; ok {
		return entry, true
	}
	if pLower := strings.ToLower(p); pLower != p {
		if entry, ok := c.qualified[pLower+"/"+norm]; ok {
			return entry, true
		}
	}
	return catalogEntry{}, false
}

func (c *Catalog) PriceFor(providerID, model string) (plugin.ModelPrice, error) {
	if c == nil {
		return plugin.ModelPrice{}, plugin.ErrPricingUnavailable
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	p := strings.TrimSpace(providerID)
	m := strings.TrimSpace(model)
	if p == "" || m == "" {
		return plugin.ModelPrice{}, plugin.ErrModelUnpriced
	}

	norm := normalize(m)
	if entry, ok := c.lookupQualified(p, norm); ok {
		return entry.price, nil
	}

	stripped := snapshotRegex.ReplaceAllString(norm, "")
	if stripped != norm && stripped != "" {
		if entry, ok := c.lookupQualified(p, stripped); ok {
			return entry.price, nil
		}
	}

	return plugin.ModelPrice{}, plugin.ErrModelUnpriced
}

func (c *Catalog) ResolveBare(model string) (string, string, plugin.ModelPrice, error) {
	if c == nil {
		return "", "", plugin.ModelPrice{}, plugin.ErrPricingUnavailable
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	model = strings.TrimSpace(model)
	if model == "" {
		return "", "", plugin.ModelPrice{}, plugin.ErrModelUnpriced
	}

	slash := strings.Index(model, "/")
	if slash > 0 {
		firstPart := model[:slash]
		rest := model[slash+1:]
		firstLower := strings.ToLower(strings.TrimSpace(firstPart))
		if c.providers[firstPart] || c.providers[firstLower] {
			if strings.TrimSpace(rest) == "" {
				return "", "", plugin.ModelPrice{}, plugin.ErrModelUnpriced
			}
			provKey := firstPart
			if !c.providers[firstPart] && c.providers[firstLower] {
				provKey = firstLower
			}
			normRest := normalize(rest)
			if entry, ok := c.lookupQualified(provKey, normRest); ok {
				return entry.providerID, entry.modelID, entry.price, nil
			}
			stripped := snapshotRegex.ReplaceAllString(normRest, "")
			if stripped != normRest && stripped != "" {
				if entry, ok := c.lookupQualified(provKey, stripped); ok {
					return entry.providerID, entry.modelID, entry.price, nil
				}
			}
			return "", "", plugin.ModelPrice{}, plugin.ErrModelUnpriced
		}
	}

	norm := normalize(model)
	candidates := c.bare[norm]
	if len(candidates) == 0 {
		stripped := snapshotRegex.ReplaceAllString(norm, "")
		if stripped != norm && stripped != "" {
			candidates = c.bare[stripped]
		}
	}

	if len(candidates) == 0 {
		return "", "", plugin.ModelPrice{}, plugin.ErrModelUnpriced
	}

	first := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if !pricesEqual(first.price, candidates[i].price) {
			return "", "", plugin.ModelPrice{}, plugin.ErrModelAmbiguous
		}
	}
	return first.providerID, first.modelID, first.price, nil
}

func (c *Catalog) ModelCount() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *Catalog) Billable() []PriceDTO {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]PriceDTO, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, PriceDTO{
			Model:       e.modelID,
			DisplayName: displayName(e),
			ProviderID:  e.providerID,
			Input:       e.price.Input,
			Output:      e.price.Output,
			Reasoning:   e.price.Reasoning,
			CacheRead:   e.price.CacheRead,
			CacheWrite:  e.price.CacheWrite,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ki := out[i].ProviderID + "/" + out[i].Model
		kj := out[j].ProviderID + "/" + out[j].Model
		return ki < kj
	})
	return out
}

func pricesEqual(a, b plugin.ModelPrice) bool {
	return a.Input == b.Input &&
		a.Output == b.Output &&
		a.Reasoning == b.Reasoning &&
		a.CacheRead == b.CacheRead &&
		a.CacheWrite == b.CacheWrite
}
