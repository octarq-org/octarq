package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/queue"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type dummyPlugin struct{}

func (d dummyPlugin) Name() string                      { return "dummy" }
func (d dummyPlugin) Models() []any                     { return nil }
func (d dummyPlugin) Mount(plugin.Mux, *plugin.Context) {}

var _ plugin.Plugin = dummyPlugin{}

func TestOverviewPluginAbsent(t *testing.T) {
	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}

	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	gdb, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	if err := gdb.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{gdb}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(gdb)
	geoResolver, _ := geo.Open("")
	taskQueue := queue.New("")

	handler := New(cfg, gdb, cipher, authMgr, geoResolver, taskQueue)
	handler.SetPlugins([]plugin.Plugin{dummyPlugin{}})
	reg := plugin.NewRegistry()
	handler.SetServiceLookup(reg.Lookup)

	srv := handler.Routes()

	cookies := loginCookies(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}

	// Keys owned by plugins should be absent when plugins are not mounted
	for _, absentKey := range []string{"mailboxes", "emails", "domains", "links"} {
		if _, exists := res[absentKey]; exists {
			t.Errorf("expected absent key %q when plugins are not mounted", absentKey)
		}
	}

	// Core keys must be present
	if _, exists := res["tokens"]; !exists {
		t.Errorf("expected core key 'tokens' to be present")
	}
}
