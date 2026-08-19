package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

func TestSettingsStore(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Drop the shared in-memory DB on cleanup so -count=2 re-runs stay isolated.
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	db.AutoMigrate(&models.Setting{})

	store := settingsStore{db: db}

	_, ok := store.Get("key1")
	if ok {
		t.Error("expected false for missing key")
	}

	if err := store.Set("key1", "val1"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, ok := store.Get("key1")
	if !ok || val != "val1" {
		t.Errorf("Get expected val1, got %q, ok=%v", val, ok)
	}

	if err := store.Set("key1", "val2"); err != nil {
		t.Fatalf("Set update failed: %v", err)
	}
	val, _ = store.Get("key1")
	if val != "val2" {
		t.Errorf("Get after update expected val2, got %q", val)
	}
}

type testDummyPlugin struct {
	name   string
	models []any
}

func (p testDummyPlugin) Name() string                      { return p.name }
func (p testDummyPlugin) Describe() plugin.Info             { return plugin.Info{Title: p.name} }
func (p testDummyPlugin) Models() []any                     { return p.models }
func (p testDummyPlugin) Menus() []plugin.MenuItem          { return nil }
func (p testDummyPlugin) Actions() []plugin.Action          { return nil }
func (p testDummyPlugin) Mount(plugin.Mux, *plugin.Context) {}

type customModelA struct {
	ID uint `gorm:"primaryKey"`
}

func (customModelA) TableName() string { return "custom_table" }

type customModelB struct {
	ID uint `gorm:"primaryKey"`
}

func (customModelB) TableName() string { return "custom_table" }

func TestPreflightCollisionsUnit(t *testing.T) {
	t.Parallel()

	// 1. Name collision error
	p1 := testDummyPlugin{name: "dupe"}
	p2 := testDummyPlugin{name: "dupe"}
	if err := preflightNameCollisions([]plugin.Plugin{p1, p2}); err == nil {
		t.Error("expected error for duplicate plugin names")
	}

	// 2. Table collision error
	pa := testDummyPlugin{name: "pA", models: []any{customModelA{}}}
	pb := testDummyPlugin{name: "pB", models: []any{customModelB{}}}
	if err := preflightTableCollisions(nil, []plugin.Plugin{pa, pb}); err == nil {
		t.Error("expected error for colliding custom table names")
	}
}

func TestGatedMuxExtra(t *testing.T) {
	t.Parallel()

	realMux := http.NewServeMux()
	g := &gatedMux{
		real:   realMux,
		plugin: "myplugin",
		enabled: func(r *http.Request, p string) (allowed, scoped bool) {
			if r.Header.Get("X-Disabled") == "true" {
				return false, true
			}
			return true, true
		},
	}

	g.HandleFunc("/test-func", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// 1. Allowed request
	req1 := httptest.NewRequest(http.MethodGet, "/test-func", nil)
	rec1 := httptest.NewRecorder()
	realMux.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("allowed request expected 200, got %d", rec1.Code)
	}

	// 2. Disabled request -> 404
	req2 := httptest.NewRequest(http.MethodGet, "/test-func", nil)
	req2.Header.Set("X-Disabled", "true")
	rec2 := httptest.NewRecorder()
	realMux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Errorf("disabled request expected 404, got %d", rec2.Code)
	}
}

type trustProxyPlugin struct {
	name        string
	serviceName string
	got         bool
}

func (p *trustProxyPlugin) Name() string             { return p.name }
func (p *trustProxyPlugin) Describe() plugin.Info    { return plugin.Info{Title: p.name} }
func (p *trustProxyPlugin) Models() []any            { return nil }
func (p *trustProxyPlugin) Menus() []plugin.MenuItem { return nil }
func (p *trustProxyPlugin) Actions() []plugin.Action { return nil }
func (p *trustProxyPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	if ctx.Provide != nil {
		ctx.Provide(p.serviceName, func(v bool) {
			p.got = v
		})
	}
}

func TestTrustProxyWiring(t *testing.T) {
	linksP := &trustProxyPlugin{name: "links", serviceName: "links.trust_proxy"}
	dnsP := &trustProxyPlugin{name: "dns", serviceName: "dns.trust_proxy"}

	services := plugin.NewRegistry()
	pctx := &plugin.Context{
		Provide: services.Provide,
		Lookup:  services.Lookup,
	}

	linksP.Mount(nil, pctx)
	dnsP.Mount(nil, pctx)

	cfgTrustProxy := true
	if fn, ok := plugin.LookupServiceAs[func(bool)](services.Lookup, "links.trust_proxy"); ok {
		fn(cfgTrustProxy)
	}
	if fn, ok := plugin.LookupServiceAs[func(bool)](services.Lookup, "dns.trust_proxy"); ok {
		fn(cfgTrustProxy)
	}

	if !linksP.got {
		t.Errorf("links plugin did not receive TrustProxy setting")
	}
	if !dnsP.got {
		t.Errorf("dns plugin did not receive TrustProxy setting")
	}
}
