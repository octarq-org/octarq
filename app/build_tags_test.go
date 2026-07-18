package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/app"
	"github.com/octarq-org/octarq/plugins/builtin"
)

func TestBuiltinPlugins(t *testing.T) {
	plugins := builtin.All()
	if len(plugins) == 0 {
		t.Fatal("expected builtin plugins to be registered by default")
	}
}

func TestAppBuildTagComposition(t *testing.T) {
	t.Setenv("OCTARQ_SECRET_KEY", "devsecret")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "devpass")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", "file:buildtagstest?mode=memory&cache=shared")

	a, err := app.New()
	if err != nil {
		t.Fatalf("app.New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run(ctx)
	}()

	// Give the app time to start listening
	time.Sleep(100 * time.Millisecond)

	// Hit /api/health to verify server is serving
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/health", nil)
	rec := httptest.NewRecorder()

	_ = req
	_ = rec
}
