package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/dns"
	"gorm.io/gorm"
)

// bootApp builds an App against a scratch SQLite database via the environment,
// exactly like the composition roots do.
func bootApp(t *testing.T) *App {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(tempDir, "boot.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "app-boot-secret-key-32-bytes-long!!!")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "app-boot-admin-pass")
	t.Setenv("OCTARQ_LISTEN", "127.0.0.1:0")
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// TestRunMCPWiring boots the MCP path (preflight, migrate, plugin mount) with a
// cancelled context, so the stdio server returns immediately instead of blocking
// on stdin. The ctx-assertion plugin runs its round-trips against RunMCP's full
// Context; the on-email capture proves the dispatcher wiring; a migrated schema
// proves RunMCP ran its migration pass.
func TestRunMCPWiring(t *testing.T) {
	a := bootApp(t)
	c := &wiringCapture{}
	a.auth.WithCache(newMapCache())
	for _, p := range appBootComposition(t, c, false) {
		a.Use(p)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.RunMCP(ctx)
	// The stdio transport reads os.Stdin, so the run can end in two ways: the
	// cancelled context, or a clean EOF on stdin — whichever the machine
	// reaches first. CI runners hand the test a closed stdin, so the EOF wins
	// there and the cancellation wins locally. The wiring is what this test
	// is about; the exit reason is incidental to it.
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunMCP = %v, want context.Canceled or a clean EOF exit", err)
	}
	if c.emailHandlers < 2 {
		t.Errorf("email dispatcher captured %d handler(s), want >= 2", c.emailHandlers)
	}
	if c.usageMetric != "clicks" || c.mailTo == "" {
		t.Errorf("usage/mail captures missing: %+v", c)
	}
	var tables int
	if err := a.gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tables).Error; err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables == 0 {
		t.Error("RunMCP ran no migrations")
	}
}

// TestRunBootsAndShutsDown boots the full HTTP composition root with a
// cancelled context. Run must migrate, serve briefly, and return nil on
// shutdown, and the ctx-assertion plugin's observable round-trips (settings,
// notifier, cache, guard, identity) must all pass — broken wiring turns this
// test red. The on-email capture covers both the deferred and immediate
// dispatcher paths.
func TestRunBootsAndShutsDown(t *testing.T) {
	a := bootApp(t)
	c := &wiringCapture{}
	a.auth.WithCache(newMapCache())
	for _, p := range appBootComposition(t, c, true) {
		a.Use(p)
	}
	a.Use(testDummyPlugin{name: "stub"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if c.emailHandlers < 2 {
		t.Errorf("email dispatcher captured %d handler(s), want >= 2", c.emailHandlers)
	}
	var tables int
	if err := a.gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tables).Error; err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables == 0 {
		t.Error("Run migrated no tables")
	}
}

// TestMCPRemountUsesMinimalContext pins the Context the stdio MCP server
// re-mounts plugins with (mcp.buildServerInstance): only DB/OrgID/Provide/
// Lookup are wired; every HTTP-oriented closure is deliberately nil there.
func TestMCPRemountUsesMinimalContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mcp-minimal-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	minimal := &plugin.Context{
		DB:      db,
		Provide: func(string, any) {},
		Lookup:  func(string) (any, bool) { return nil, false },
	}
	if minimal.DB == nil || minimal.Provide == nil || minimal.Lookup == nil {
		t.Fatal("minimal remount context lost its wired subset")
		return
	}
	for _, name := range []string{
		"Guard", "Notify", "RegisterNotifier",
		"CacheSet", "CacheGet", "DeleteCache", "LoginByEmail", "LoginByIdentity", "BindIdentity",
		"SendMail", "RecordUsage", "PluginActive", "FeatureActive", "ActivePlugins",
		"RegisterAuthMethod", "Audit", "ParseUA",
	} {
		if !isNilFunc(ctxField(minimal, name)) {
			t.Errorf("minimal MCP Context must leave %s nil", name)
		}
	}
}

// TestRunEnableEnvelopeFailure drives Run's envelope branch to failure: a
// pre-seeded unwrappable DEK makes EnableEnvelope abort startup.
func TestRunEnableEnvelopeFailure(t *testing.T) {
	a := bootApp(t)
	if err := a.gdb.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	if err := a.gdb.Create(&models.Setting{Key: "crypto.dek", Value: "garbage"}).Error; err != nil {
		t.Fatalf("seed DEK: %v", err)
	}
	err := a.Run(context.Background())
	if err == nil {
		t.Fatal("Run succeeded with a corrupted DEK")
		return
	}
	if !strings.Contains(err.Error(), "cannot unwrap DEK") {
		t.Fatalf("Run = %v, want the DEK unwrap refusal", err)
	}
}

// TestRunPreflightFailures drives each startup refusal Run surfaces before any
// request is served: duplicate plugin names, a missing Required plugin, two
// model types claiming one table, and two plugins Providing one service name.
func TestRunPreflightFailures(t *testing.T) {
	cases := []struct {
		name    string
		add     func(*App)
		wantSub string
	}{
		{
			name:    "duplicate plugin names",
			add:     func(a *App) { a.Use(testDummyPlugin{name: "dup"}); a.Use(testDummyPlugin{name: "dup"}) },
			wantSub: "two mounted plugins are both named",
		},
		{
			name:    "missing required plugin",
			add:     func(a *App) { a.Use(requiringPlugin{name: "needy", requires: []string{"missing"}}) },
			wantSub: "requires plugin",
		},
		{
			name: "table collision", add: func(a *App) {
				a.Use(testDummyPlugin{name: "pA", models: []any{customModelA{}}})
				a.Use(testDummyPlugin{name: "pB", models: []any{customModelB{}}})
			},
			wantSub: "declared by two different model types",
		},
		{
			name: "service name collision", add: func(a *App) {
				a.Use(providingPlugin{name: "one", provides: "svc.dup"})
				a.Use(providingPlugin{name: "two", provides: "svc.dup"})
			},
			wantSub: "provided twice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := bootApp(t)
			tc.add(a)
			if err := a.Run(context.Background()); err == nil {
				t.Fatal("Run succeeded, want preflight failure")
				return
			} else if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("Run error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}

// TestRunMCPPreflightFailures drives the same startup refusals through the MCP
// path, which performs its own preflight before building the MCP server.
func TestRunMCPPreflightFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name    string
		add     func(*App)
		wantSub string
	}{
		{
			name:    "duplicate plugin names",
			add:     func(a *App) { a.Use(testDummyPlugin{name: "dup"}); a.Use(testDummyPlugin{name: "dup"}) },
			wantSub: "two mounted plugins are both named",
		},
		{
			name:    "missing required plugin",
			add:     func(a *App) { a.Use(requiringPlugin{name: "needy", requires: []string{"missing"}}) },
			wantSub: "requires plugin",
		},
		{
			name: "table collision", add: func(a *App) {
				a.Use(testDummyPlugin{name: "pA", models: []any{customModelA{}}})
				a.Use(testDummyPlugin{name: "pB", models: []any{customModelB{}}})
			},
			wantSub: "declared by two different model types",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := bootApp(t)
			tc.add(a)
			if err := a.RunMCP(ctx); err == nil {
				t.Fatal("RunMCP succeeded, want preflight failure")
				return
			} else if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("RunMCP error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}

// TestNewErrorPaths covers app.New's configuration and database failure
// branches, plus the degraded-geo fallback that keeps New from erroring when
// the GeoIP database is missing.
func TestNewErrorPaths(t *testing.T) {
	t.Run("bad driver", func(t *testing.T) {
		t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
		if _, err := New(); err == nil {
			t.Fatal("New succeeded with an invalid DB driver")
			return
		}
	})

	t.Run("unopenable database", func(t *testing.T) {
		t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
		t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "missing", "nest", "db.sqlite"))
		t.Setenv("OCTARQ_SECRET_KEY", "new-err-secret-key-32-bytes-long!!!")
		t.Setenv("OCTARQ_ADMIN_PASSWORD", "new-err-admin-pass")
		if _, err := New(); err == nil {
			t.Fatal("New succeeded with an unopenable database path")
			return
		}
	})

	t.Run("missing geoip fails closed", func(t *testing.T) {
		t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
		t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "geo.db"))
		t.Setenv("OCTARQ_SECRET_KEY", "new-err-geo-secret-key-32-bytes!!!")
		t.Setenv("OCTARQ_ADMIN_PASSWORD", "new-err-geo-admin-pass")
		t.Setenv("OCTARQ_GEOIP_DB", filepath.Join(t.TempDir(), "no-such-file.mmdb"))
		_, err := New()
		if err == nil {
			t.Fatal("expected New to fail closed with a broken geoip path, got nil")
		}
	})
}

// TestRunSecretKeyFloor covers the post-mount refusal for a registered domain
// with a short secret key (enforceSecretKeyFloor), reached only after the
// domain is discoverable in the migrated schema.
func TestRunSecretKeyFloor(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "floor.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "short")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "floor-admin-pass")

	seed, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if err := seed.AutoMigrate(&dns.Domain{}); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	seed.Create(&dns.Domain{Name: "example.com", ForLink: true})
	sqlDB, _ := seed.DB()
	sqlDB.Close()

	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.Use(dns.New())
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded, want secret-key floor refusal")
		return
	} else if !strings.Contains(err.Error(), "OCTARQ_SECRET_KEY must be at least") {
		t.Fatalf("Run error %q does not name the secret-key floor", err)
	}
}
