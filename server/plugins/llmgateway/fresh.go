package llmgateway

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

var (
	nowFunc          = time.Now
	fetchFunc        func(ctx context.Context) ([]byte, error)
	pricingDevURL    = "https://models.dev/api.json"
	cacheDirOverride = ""
	defaultRefresher = &refresher{}
)

type refresher struct {
	mu          sync.Mutex
	lastAttempt time.Time
	inFlight    bool
	wg          sync.WaitGroup
}

type modelsDevCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
	Reasoning  *float64 `json:"reasoning"`
}

type modelsDevModel struct {
	Name string         `json:"name"`
	Cost *modelsDevCost `json:"cost"`
}

type modelsDevProvider struct {
	Models map[string]modelsDevModel `json:"models"`
}

func BundleFromModelsDev(raw []byte) ([]byte, error) {
	var providers map[string]modelsDevProvider
	if err := json.Unmarshal(raw, &providers); err != nil {
		return nil, fmt.Errorf("llm gateway: parse models.dev json: %w", err)
	}
	if len(providers) == 0 {
		return nil, errors.New("llm gateway: models.dev json has no providers")
	}

	pids := make([]string, 0, len(providers))
	for pid := range providers {
		pids = append(pids, pid)
	}
	sort.Strings(pids)

	var rows []bundleRow
	for _, pid := range pids {
		prov := providers[pid]
		mids := make([]string, 0, len(prov.Models))
		for mid := range prov.Models {
			mids = append(mids, mid)
		}
		sort.Strings(mids)

		for _, mid := range mids {
			m := prov.Models[mid]
			if m.Cost == nil || m.Cost.Input == nil || m.Cost.Output == nil {
				continue
			}
			if *m.Cost.Input < 0 || *m.Cost.Output < 0 {
				continue
			}
			var cr, cw, re float64
			if m.Cost.CacheRead != nil && *m.Cost.CacheRead >= 0 {
				cr = *m.Cost.CacheRead
			}
			if m.Cost.CacheWrite != nil && *m.Cost.CacheWrite >= 0 {
				cw = *m.Cost.CacheWrite
			}
			if m.Cost.Reasoning != nil && *m.Cost.Reasoning >= 0 {
				re = *m.Cost.Reasoning
			}
			rows = append(rows, bundleRow{
				modelID:    mid,
				providerID: pid,
				input:      *m.Cost.Input,
				output:     *m.Cost.Output,
				cacheRead:  cr,
				cacheWrite: cw,
				reasoning:  re,
				name:       strings.TrimSpace(m.Name),
			})
		}
	}

	if len(rows) == 0 {
		return nil, errors.New("llm gateway: models.dev json yielded 0 valid models")
	}

	b := bundleFile{
		GeneratedAt: nowFunc().UTC().Format(time.RFC3339),
		Source:      "https://models.dev/api.json",
		Models:      rows,
	}

	var buf bytes.Buffer
	gw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("llm gateway: create gzip writer: %w", err)
	}
	if err := json.NewEncoder(gw).Encode(b); err != nil {
		return nil, fmt.Errorf("llm gateway: encode bundle json: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("llm gateway: close gzip writer: %w", err)
	}
	return buf.Bytes(), nil
}

func ParseBundleGz(gzData []byte) (*Catalog, error) {
	gr, err := gzip.NewReader(bytes.NewReader(gzData))
	if err != nil {
		return nil, fmt.Errorf("llm gateway: decompress catalog: %w", err)
	}
	defer gr.Close()

	var b bundleFile
	if err := json.NewDecoder(gr).Decode(&b); err != nil {
		return nil, fmt.Errorf("llm gateway: parse price catalog: %w", err)
	}
	if len(b.Models) == 0 {
		return nil, errors.New("llm gateway: price catalog holds no models")
	}

	cat := &Catalog{
		generatedAt: b.GeneratedAt,
		source:      b.Source,
		entries:     make([]catalogEntry, 0, len(b.Models)),
		qualified:   make(map[string]catalogEntry, len(b.Models)),
		bare:        make(map[string][]catalogEntry, len(b.Models)),
		providers:   make(map[string]bool),
	}

	for _, row := range b.Models {
		entry := catalogEntry{
			modelID:    row.modelID,
			providerID: row.providerID,
			name:       row.name,
			price: plugin.ModelPrice{
				Input:      row.input,
				Output:     row.output,
				Reasoning:  row.reasoning,
				CacheRead:  row.cacheRead,
				CacheWrite: row.cacheWrite,
			},
		}
		cat.entries = append(cat.entries, entry)
		cat.providers[row.providerID] = true
		cat.providers[strings.ToLower(strings.TrimSpace(row.providerID))] = true

		normModel := normalize(row.modelID)
		qKey := row.providerID + "/" + normModel
		cat.qualified[qKey] = entry
		if pLower := strings.ToLower(strings.TrimSpace(row.providerID)); pLower != row.providerID {
			cat.qualified[pLower+"/"+normModel] = entry
		}
		cat.bare[normModel] = append(cat.bare[normModel], entry)
	}

	return cat, nil
}

func diskCachePath() string {
	if cacheDirOverride != "" {
		return filepath.Join(cacheDirOverride, "octarq", "models.json.gz")
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "octarq", "models.json.gz")
}

func loadDiskCache() *Catalog {
	cp := diskCachePath()
	if cp == "" {
		return nil
	}
	data, err := os.ReadFile(cp)
	if err != nil {
		return nil
	}
	cat, err := ParseBundleGz(data)
	if err != nil {
		slog.Warn("llm gateway: cached models.json.gz corrupted, deleting", "error", err)
		_ = os.Remove(cp)
		return nil
	}
	return cat
}

func saveDiskCache(gzData []byte) {
	cp := diskCachePath()
	if cp == "" {
		slog.Warn("llm gateway: user cache directory unavailable")
		return
	}
	cacheDir := filepath.Dir(cp)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		slog.Warn("llm gateway: create cache directory failed", "error", err)
		return
	}
	tmpFile := cp + ".tmp." + strconv.FormatInt(nowFunc().UnixNano(), 10)
	if err := os.WriteFile(tmpFile, gzData, 0644); err != nil {
		slog.Warn("llm gateway: write cache file failed", "error", err)
		return
	}
	if err := os.Rename(tmpFile, cp); err != nil {
		slog.Warn("llm gateway: rename cache file failed", "error", err)
		_ = os.Remove(tmpFile)
	}
}

func fetchModelsDev(ctx context.Context) ([]byte, error) {
	if fetchFunc != nil {
		return fetchFunc(ctx)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pricingDevURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (r *refresher) refresh(ctx context.Context) error {
	raw, err := fetchModelsDev(ctx)
	if err != nil {
		slog.Warn("llm gateway: fetch models.dev failed", "error", err)
		return err
	}
	gzData, err := BundleFromModelsDev(raw)
	if err != nil {
		slog.Warn("llm gateway: bundle models.dev failed", "error", err)
		return err
	}
	newCat, err := ParseBundleGz(gzData)
	if err != nil {
		slog.Warn("llm gateway: parse bundled models.dev failed", "error", err)
		return err
	}
	replaceCatalog(newCat)
	saveDiskCache(gzData)
	return nil
}

func triggerUnpricedRefresh() {
	defaultRefresher.triggerUnpriced()
}

func (r *refresher) triggerUnpriced() {
	if strings.EqualFold(os.Getenv("OCTARQ_PRICING_REFRESH"), "off") {
		return
	}
	r.mu.Lock()
	now := nowFunc()
	if !r.lastAttempt.IsZero() && now.Sub(r.lastAttempt) < 6*time.Hour {
		r.mu.Unlock()
		return
	}
	if r.inFlight {
		r.mu.Unlock()
		return
	}
	r.lastAttempt = now
	r.inFlight = true
	r.mu.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() {
			r.mu.Lock()
			r.inFlight = false
			r.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = r.refresh(ctx)
	}()
}

func startPricingRefresher(ctx context.Context) {
	seed, _ := initSeedCatalog()
	if seed != nil {
		replaceCatalog(seed)
	}
	diskCat := loadDiskCache()
	if diskCat != nil && seed != nil {
		tDisk, err1 := time.Parse(time.RFC3339, diskCat.generatedAt)
		tSeed, err2 := time.Parse(time.RFC3339, seed.generatedAt)
		if err1 == nil && err2 == nil && tDisk.After(tSeed) {
			replaceCatalog(diskCat)
		}
	} else if diskCat != nil && seed == nil {
		replaceCatalog(diskCat)
	}

	if strings.EqualFold(os.Getenv("OCTARQ_PRICING_REFRESH"), "off") {
		return
	}

	current := loadCatalog()
	needImmediate := true
	if current != nil && current.generatedAt != "" {
		if genTime, err := time.Parse(time.RFC3339, current.generatedAt); err == nil {
			if nowFunc().Sub(genTime) < 24*time.Hour {
				needImmediate = false
			}
		}
	}

	if needImmediate {
		_ = defaultRefresher.refresh(ctx)
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = defaultRefresher.refresh(ctx)
		}
	}
}
