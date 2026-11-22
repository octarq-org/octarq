package llmgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFreshHTTP500PreservesCatalog(t *testing.T) {
	initialJSON := `{
		"generated_at": "2026-11-22T00:00:00Z",
		"source": "https://models.dev/api.json",
		"models": [["old-model", "anthropic", 1, 2, 0, 0, 0,""]]
	}`
	oldCat, err := ParseBundleGz(makeGzBundle(t, initialJSON))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	setCatalogForTest(t, oldCat)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	setRefresherForTest(t, nil, nil, t.TempDir(), ts.URL)

	err = defaultRefresher.refresh(context.Background())
	if err == nil {
		t.Fatal("expected refresh error, got nil")
	}

	current := loadCatalog()
	if current == nil {
		t.Fatal("current catalog is nil")
	}
	p, err := current.PriceFor("anthropic", "old-model")
	if err != nil || p.Input != 1 {
		t.Fatalf("expected old catalog preserved, got price=%+v err=%v", p, err)
	}
}

func TestFreshSuccessReplacesCatalog(t *testing.T) {
	initialJSON := `{
		"generated_at": "2026-11-22T00:00:00Z",
		"source": "https://models.dev/api.json",
		"models": [["old-model", "anthropic", 1, 2, 0, 0, 0,""]]
	}`
	oldCat, _ := ParseBundleGz(makeGzBundle(t, initialJSON))
	setCatalogForTest(t, oldCat)

	newRawJSON := `{
		"anthropic": {
			"models": {
				"new-model": {
					"cost": {
						"input": 10,
						"output": 20,
						"cache_read": 1,
						"cache_write": 2,
						"reasoning": 5
					}
				}
			}
		}
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(newRawJSON))
	}))
	defer ts.Close()

	tempDir := t.TempDir()
	setRefresherForTest(t, nil, nil, tempDir, ts.URL)

	err := defaultRefresher.refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	current := loadCatalog()
	p, err := current.PriceFor("anthropic", "new-model")
	if err != nil || p.Input != 10 || p.Output != 20 {
		t.Fatalf("expected new catalog with new-model, got %+v, err=%v", p, err)
	}

	cacheFile := filepath.Join(tempDir, "octarq", "models.json.gz")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("expected cache file written at %s: %v", cacheFile, err)
	}
}

func TestFreshRefreshOffEnv(t *testing.T) {
	t.Setenv("OCTARQ_PRICING_REFRESH", "off")

	var called atomic.Bool
	setRefresherForTest(t, nil, func(ctx context.Context) ([]byte, error) {
		called.Store(true)
		return nil, nil
	}, t.TempDir(), "")

	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return immediately when refresh is off")
	}

	if called.Load() {
		t.Fatal("refresher called network despite OCTARQ_PRICING_REFRESH=off")
	}
}

func TestFreshCorruptedDiskCacheFallsBackToSeed(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "octarq")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	corruptFile := filepath.Join(cacheDir, "models.json.gz")
	if err := os.WriteFile(corruptFile, []byte("this is not valid gzip data"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	setRefresherForTest(t, nil, nil, tempDir, "")
	t.Setenv("OCTARQ_PRICING_REFRESH", "off")

	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)

	if _, err := os.Stat(corruptFile); !os.IsNotExist(err) {
		t.Fatalf("corrupt file should be removed, got err: %v", err)
	}

	cat := loadCatalog()
	if cat == nil {
		t.Fatal("loadCatalog should return seed catalog when disk cache is bad")
	}
}

func TestFreshUnwritableCacheDirDoesNotPanic(t *testing.T) {
	tempDir := t.TempDir()
	badPath := filepath.Join(tempDir, "octarq")
	if err := os.WriteFile(badPath, []byte("blocking-file"), 0444); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	setRefresherForTest(t, nil, nil, badPath, "")

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("saveDiskCache panicked: %v", r)
		}
	}()

	saveDiskCache([]byte("some data"))
}

func TestFreshUnpricedAccelerationThrottle(t *testing.T) {
	var fetchCount atomic.Int32
	var fakeNowMu sync.Mutex
	fakeNow := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

	tempDir := t.TempDir()
	setRefresherForTest(t, func() time.Time {
		fakeNowMu.Lock()
		defer fakeNowMu.Unlock()
		return fakeNow
	}, func(ctx context.Context) ([]byte, error) {
		fetchCount.Add(1)
		return []byte(`{"anthropic":{"models":{"claude-opus-4-8":{"cost":{"input":5,"output":25,"cache_read":0.5,"cache_write":6.25}}}}}`), nil
	}, tempDir, "")

	triggerUnpricedRefresh()
	time.Sleep(100 * time.Millisecond)
	if fetchCount.Load() != 1 {
		t.Fatalf("first trigger count = %d, want 1", fetchCount.Load())
	}

	fakeNowMu.Lock()
	fakeNow = fakeNow.Add(2 * time.Hour)
	fakeNowMu.Unlock()
	triggerUnpricedRefresh()
	time.Sleep(100 * time.Millisecond)
	if fetchCount.Load() != 1 {
		t.Fatalf("trigger before 6h count = %d, want 1", fetchCount.Load())
	}

	fakeNowMu.Lock()
	fakeNow = fakeNow.Add(5 * time.Hour)
	fakeNowMu.Unlock()
	triggerUnpricedRefresh()
	time.Sleep(100 * time.Millisecond)
	if fetchCount.Load() != 2 {
		t.Fatalf("trigger after 7h count = %d, want 2", fetchCount.Load())
	}
}

func TestBundleFromModelsDevRefusesEmptyInputs(t *testing.T) {
	if _, err := BundleFromModelsDev([]byte(`{}`)); err == nil {
		t.Fatal("empty provider map accepted")
	}
	if _, err := BundleFromModelsDev([]byte(`{"p":{"models":{"m":{"name":"M"}}}}`)); err == nil {
		t.Fatal("provider with no priced model accepted")
	}
}

func TestFreshBadBodyKeepsCatalog(t *testing.T) {
	initialJSON := `{
		"generated_at": "2026-11-22T00:00:00Z",
		"source": "https://models.dev/api.json",
		"models": [["old-model", "anthropic", 1, 2, 0, 0, 0, "Old"]]
	}`
	oldCat, err := ParseBundleGz(makeGzBundle(t, initialJSON))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	setCatalogForTest(t, oldCat)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer ts.Close()

	setRefresherForTest(t, nil, nil, t.TempDir(), ts.URL)
	if err := defaultRefresher.refresh(context.Background()); err == nil {
		t.Fatal("expected a parse error")
	}
	if _, err := loadCatalog().PriceFor("anthropic", "old-model"); err != nil {
		t.Fatalf("old catalog was lost: %v", err)
	}
}

func TestDiskCachePathUsesTheOverride(t *testing.T) {
	dir := t.TempDir()
	setRefresherForTest(t, nil, nil, dir, "")
	want := filepath.Join(dir, "octarq", "models.json.gz")
	if got := diskCachePath(); got != want {
		t.Fatalf("diskCachePath = %q, want %q", got, want)
	}
}

func TestStartPricingRefresherRefreshesAndStops(t *testing.T) {
	var calls atomic.Int32
	body := `{"anthropic":{"models":{"fresh-model":{"name":"Fresh","cost":{"input":1,"output":2}}}}}`
	later := func() time.Time { return time.Now().Add(48 * time.Hour) }
	setRefresherForTest(t, later, func(context.Context) ([]byte, error) {
		calls.Add(1)
		return []byte(body), nil
	}, t.TempDir(), "")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		startPricingRefresher(ctx)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() == 0 {
		cancel()
		t.Fatal("the startup refresh did not run")
	}
	if _, err := loadCatalog().PriceFor("anthropic", "fresh-model"); err != nil {
		cancel()
		t.Fatalf("fresh catalog was not installed: %v", err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startPricingRefresher did not stop after cancel")
	}
}

func TestDiskCachePathFallsBackToTheUserCacheDir(t *testing.T) {
	setRefresherForTest(t, nil, nil, "", "")
	got := diskCachePath()
	if got == "" || !strings.HasSuffix(got, filepath.Join("octarq", "models.json.gz")) {
		t.Fatalf("diskCachePath = %q, want the user cache dir", got)
	}
}

func TestStartPricingRefresherPrefersANewerDiskCache(t *testing.T) {
	dir := t.TempDir()
	body := `{"generated_at":"2030-01-01T00:00:00Z","source":"https://models.dev/api.json","models":[["disk-model","anthropic",7,8,0,0,0,"Disk"]]}`
	cacheDir := filepath.Join(dir, "octarq")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "models.json.gz"), makeGzBundle(t, body), 0644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	t.Setenv("OCTARQ_PRICING_REFRESH", "off")
	setRefresherForTest(t, nil, nil, dir, "")
	startPricingRefresher(context.Background())

	price, err := loadCatalog().PriceFor("anthropic", "disk-model")
	if err != nil {
		t.Fatalf("PriceFor: %v", err)
	}
	if price.Input != 7 || price.Output != 8 {
		t.Fatalf("price = %+v, want the disk cache 7/8", price)
	}
}

func TestBundleFromModelsDevTreatsNegativeOptionalCostsAsZero(t *testing.T) {
	raw := []byte(`{"p":{"models":{"m":{"name":"M","cost":{"input":1,"output":2,"cache_read":-1,"cache_write":-1,"reasoning":-1}}}}}`)
	gz, err := BundleFromModelsDev(raw)
	if err != nil {
		t.Fatalf("BundleFromModelsDev: %v", err)
	}
	cat, err := ParseBundleGz(gz)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	price, err := cat.PriceFor("p", "m")
	if err != nil {
		t.Fatalf("PriceFor: %v", err)
	}
	if price.CacheRead != 0 || price.CacheWrite != 0 || price.Reasoning != 0 {
		t.Fatalf("price = %+v, want zero optional rates", price)
	}
}

func TestFreshConnectionErrorKeepsCatalog(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := ts.URL
	ts.Close()

	setRefresherForTest(t, nil, nil, t.TempDir(), url)
	if err := defaultRefresher.refresh(context.Background()); err == nil {
		t.Fatal("expected a connection error")
	}
}
