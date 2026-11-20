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

	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "octarq_http_requests_total",
			Help: "Total number of HTTP requests processed by Octarq.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "octarq_http_request_duration_seconds",
			Help:    "Latency of HTTP requests in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	MCPActiveSessions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "octarq_mcp_active_sessions",
			Help: "Current active MCP sessions.",
		},
	)

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

	HTTPRequestsTotal.WithLabelValues("GET", "/metrics", "200")
	HTTPRequestDuration.WithLabelValues("GET", "/metrics")
}

func DefaultPrometheusRegistry() *prometheus.Registry {
	return defaultRegistry
}

func DefaultPrometheusHandler() http.Handler {
	inner := promhttp.HandlerFor(defaultRegistry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateDBStats()
		inner.ServeHTTP(w, r)
	})
}

func RecordHTTPRequest(method, path string, status int, duration time.Duration) {
	statusStr := strconv.Itoa(status)
	HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func IncMCPSessions() {
	MCPActiveSessions.Inc()
}

func DecMCPSessions() {
	MCPActiveSessions.Dec()
}

func SetMCPSessions(n int) {
	MCPActiveSessions.Set(float64(n))
}

func SetDBOpenConnections(n int) {
	DBOpenConnections.Set(float64(n))
}

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
