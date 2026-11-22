package llmgateway

import (
	"context"
	"testing"
	"time"
)

func setCatalogForTest(t *testing.T, c *Catalog) {
	t.Helper()
	catalogMu.Lock()
	prev := globalCatalog
	globalCatalog = c
	catalogMu.Unlock()
	t.Cleanup(func() {
		catalogMu.Lock()
		if prev != nil {
			globalCatalog = prev
		} else {
			globalCatalog = seedCatalog
		}
		catalogMu.Unlock()
	})
}

func setRefresherForTest(t *testing.T, now func() time.Time, fetch func(ctx context.Context) ([]byte, error), cacheDir string, devURL string) {
	t.Helper()
	prevNow := nowFunc
	prevFetch := fetchFunc
	prevCacheDir := cacheDirOverride
	prevURL := pricingDevURL
	prevRefresher := defaultRefresher

	catalogMu.RLock()
	prevCatalog := globalCatalog
	catalogMu.RUnlock()

	defaultRefresher = &refresher{}
	if now != nil {
		nowFunc = now
	}
	if fetch != nil {
		fetchFunc = fetch
	}
	if cacheDir != "" {
		cacheDirOverride = cacheDir
	}
	if devURL != "" {
		pricingDevURL = devURL
	}

	t.Cleanup(func() {
		defaultRefresher.wg.Wait()
		nowFunc = prevNow
		fetchFunc = prevFetch
		cacheDirOverride = prevCacheDir
		pricingDevURL = prevURL
		defaultRefresher = prevRefresher
		if prevCatalog != nil {
			replaceCatalog(prevCatalog)
		} else {
			seed, _ := initSeedCatalog()
			replaceCatalog(seed)
		}
	})
}
