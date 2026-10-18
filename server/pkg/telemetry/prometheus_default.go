package telemetry

import (
	"database/sql"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	defaultRegistry = prometheus.NewRegistry()

	// HTTPRequestsTotal tracks total number of HTTP requests.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "octarq_http_requests_total",
			Help: "Total number of HTTP requests processed by Octarq.",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration tracks latency of HTTP requests.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "octarq_http_request_duration_seconds",
			Help:    "Latency of HTTP requests in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	// MCPActiveSessions tracks current active MCP connections.
	MCPActiveSessions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octarq_mcp_active_sessions",
			Help: "Current active MCP sessions.",
		},
	)

	// DBOpenConnections tracks database connection pool open connections.
	DBOpenConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octarq_db_open_connections",
			Help: "Current database connection pool open connections.",
		},
	)

	dbMu       sync.RWMutex
	trackedDBs []*sql.DB
)

func init() {
	defaultRegistry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		HTTPRequestsTotal,
		HTTPRequestDuration,
		MCPActiveSessions,
		DBOpenConnections,
	)

	// Pre-populate default series so `# HELP` and `# TYPE` headers are
	// immediately present in exposition output before any external requests.
	HTTPRequestsTotal.WithLabelValues("GET", "/metrics", "200")
	HTTPRequestDuration.WithLabelValues("GET", "/metrics")
}

// DefaultPrometheusRegistry returns the default lightweight Prometheus registry.
func DefaultPrometheusRegistry() *prometheus.Registry {
	return defaultRegistry
}

// DefaultPrometheusHandler returns an http.Handler that serves metrics from
// the default lightweight Prometheus registry, refreshing DB connection stats
// on scrape.
func DefaultPrometheusHandler() http.Handler {
	inner := promhttp.HandlerFor(defaultRegistry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateDBStats()
		inner.ServeHTTP(w, r)
	})
}

// RecordHTTPRequest records an HTTP request to the default Prometheus metrics.
func RecordHTTPRequest(method, path string, status int, duration time.Duration) {
	statusStr := strconv.Itoa(status)
	HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

// IncMCPSessions increments active MCP session count.
func IncMCPSessions() {
	MCPActiveSessions.Inc()
}

// DecMCPSessions decrements active MCP session count.
func DecMCPSessions() {
	MCPActiveSessions.Dec()
}

// SetMCPSessions sets active MCP session count.
func SetMCPSessions(n int) {
	MCPActiveSessions.Set(float64(n))
}

// SetDBOpenConnections sets open DB connection gauge.
func SetDBOpenConnections(n int) {
	DBOpenConnections.Set(float64(n))
}

// RegisterDB registers a database connection pool to have its open connections tracked.
func RegisterDB(db *sql.DB) {
	if db == nil {
		return
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	trackedDBs = append(trackedDBs, db)
}

func updateDBStats() {
	dbMu.RLock()
	defer dbMu.RUnlock()
	if len(trackedDBs) == 0 {
		return
	}
	var total int
	for _, db := range trackedDBs {
		total += db.Stats().OpenConnections
	}
	DBOpenConnections.Set(float64(total))
}
