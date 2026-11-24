package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newCharacterizationApp(t *testing.T) *App {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "app_char.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-16-bytes")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-pass")

	a, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return a
}

func TestCharacterization_AppLifecycleAndPlugins(t *testing.T) {
	a := newCharacterizationApp(t)

	if a.DB() == nil {
		t.Fatal("expected non-nil DB() from initialized App")
	}

	if len(a.Plugins()) != 0 {
		t.Fatalf("expected 0 plugins initially, got %d", len(a.Plugins()))
	}

	p := testDummyPlugin{name: "test-plugin"}
	a.Use(p)

	if len(a.Plugins()) != 1 {
		t.Fatalf("expected 1 plugin after Use(), got %d", len(a.Plugins()))
	}
	if a.Plugins()[0].Name() != "test-plugin" {
		t.Fatalf("expected plugin name 'test-plugin', got %q", a.Plugins()[0].Name())
	}
}

func TestCharacterization_GatedRouterBehavior(t *testing.T) {
	mux := http.NewServeMux()
	gm := &gatedMux{
		real:   mux,
		plugin: "test-plugin",
		enabled: func(_ *http.Request, _ string) (bool, bool) {
			return true, true
		},
	}
	gm.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from gated request, got %d", rec.Code)
	}
}
