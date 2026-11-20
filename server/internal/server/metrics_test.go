package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

const testMetricsToken = "metrics-token-for-tests"

func newMetricsTokenMiddleware() *middleware {
	return newMiddleware(RuntimeSettings{MetricsToken: func() string { return testMetricsToken }})
}

func TestMetrics_DefaultPrometheusFormat(t *testing.T) {
	mw := newMetricsTokenMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer "+testMetricsToken)
	rec := httptest.NewRecorder()

	mw.handle(rec, req, okHandler)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain Content-Type, got %q", ct)
	}

	body := rec.Body.String()

	goGoroutinesRegex := regexp.MustCompile(`# HELP go_goroutines`)
	if !goGoroutinesRegex.MatchString(body) {
		t.Errorf("expected body to contain '# HELP go_goroutines', body:\n%s", body)
	}

	if !strings.Contains(body, "octarq_http_requests_total") {
		t.Errorf("expected body to contain 'octarq_http_requests_total', body:\n%s", body)
	}
	if !strings.Contains(body, "octarq_http_request_duration_seconds") {
		t.Errorf("expected body to contain 'octarq_http_request_duration_seconds', body:\n%s", body)
	}
	if !strings.Contains(body, "octarq_mcp_active_sessions") {
		t.Errorf("expected body to contain 'octarq_mcp_active_sessions', body:\n%s", body)
	}
	if !strings.Contains(body, "octarq_db_open_connections") {
		t.Errorf("expected body to contain 'octarq_db_open_connections', body:\n%s", body)
	}
}

func TestMetrics_SecurityInterception(t *testing.T) {
	t.Run("NoTokenConfigured", func(t *testing.T) {
		mw := newMiddleware(RuntimeSettings{})
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = "198.51.100.23:54321"
		rec := httptest.NewRecorder()

		mw.handle(rec, req, okHandler)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("no token configured: want 404 Not Found, got %d", rec.Code)
		}
	})

	t.Run("LoopbackPeerWithoutToken", func(t *testing.T) {
		mw := newMetricsTokenMiddleware()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		rec := httptest.NewRecorder()

		mw.handle(rec, req, okHandler)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("loopback request without token: want 403 Forbidden, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "forbidden: send the OCTARQ_METRICS_TOKEN bearer token") {
			t.Errorf("expected forbidden message, got %q", rec.Body.String())
		}
	})

	t.Run("LoopbackPeerWithToken", func(t *testing.T) {
		mw := newMetricsTokenMiddleware()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Header.Set("Authorization", "Bearer "+testMetricsToken)
		rec := httptest.NewRecorder()

		mw.handle(rec, req, okHandler)

		if rec.Code != http.StatusOK {
			t.Fatalf("loopback request with the token: want 200 OK, got %d", rec.Code)
		}
	})

	t.Run("RemoteWithWrongToken", func(t *testing.T) {
		mw := newMetricsTokenMiddleware()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = "198.51.100.23:54321"
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()

		mw.handle(rec, req, okHandler)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("remote request with invalid token: want 403 Forbidden, got %d", rec.Code)
		}
	})

	t.Run("RemoteWithCorrectSettingsToken", func(t *testing.T) {
		mw := newMetricsTokenMiddleware()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = "198.51.100.23:54321"
		req.Header.Set("Authorization", "Bearer "+testMetricsToken)
		rec := httptest.NewRecorder()

		mw.handle(rec, req, okHandler)

		if rec.Code != http.StatusOK {
			t.Fatalf("remote request with correct token: want 200 OK, got %d", rec.Code)
		}
	})

	t.Run("RemoteWithEnvToken", func(t *testing.T) {
		t.Setenv("OCTARQ_METRICS_TOKEN", "env-token-987")
		mw := newMiddleware(RuntimeSettings{})
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = "198.51.100.23:54321"
		req.Header.Set("Authorization", "Bearer env-token-987")
		rec := httptest.NewRecorder()

		mw.handle(rec, req, okHandler)

		if rec.Code != http.StatusOK {
			t.Fatalf("remote request with correct env token: want 200 OK, got %d", rec.Code)
		}
	})
}

func TestMetrics_JSONFormatCompatibility(t *testing.T) {
	mw := newMetricsTokenMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/metrics?format=json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer "+testMetricsToken)
	rec := httptest.NewRecorder()

	mw.handle(rec, req, okHandler)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected application/json Content-Type, got %q", ct)
	}

	var data map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("failed to decode JSON metrics snapshot: %v", err)
	}

	expectedKeys := []string{
		"uptime_seconds",
		"requests_total",
		"responses_2xx",
		"responses_3xx",
		"responses_4xx",
		"responses_5xx",
		"requests_in_flight",
	}

	for _, k := range expectedKeys {
		if _, ok := data[k]; !ok {
			t.Errorf("missing expected key %q in JSON metrics snapshot: %+v", k, data)
		}
	}
}

func TestMetrics_Disabled(t *testing.T) {
	t.Setenv("OCTARQ_METRICS_ENABLED", "false")
	mw := newMiddleware(RuntimeSettings{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	mw.handle(rec, req, okHandler)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when metrics disabled, got %d", rec.Code)
	}
}
