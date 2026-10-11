package server

import (
	"expvar"
	"time"
)

// Metrics (stdlib expvar)

// metrics holds the operational counters exposed at /metrics. They are backed
// by expvar so they are safe for concurrent updates without extra locking.
type metrics struct {
	startedAt time.Time
	requests  *expvar.Int
	status2xx *expvar.Int
	status3xx *expvar.Int
	status4xx *expvar.Int
	status5xx *expvar.Int
	inFlight  *expvar.Int
}

// newMetrics builds the counters. expvar's global registry panics on duplicate
// names, so we look up existing vars first (handy across test re-runs) and fall
// back to publishing fresh ones.
func newMetrics() *metrics {
	return &metrics{
		startedAt: time.Now(),
		requests:  getOrNewInt("octarq_requests_total"),
		status2xx: getOrNewInt("octarq_responses_2xx"),
		status3xx: getOrNewInt("octarq_responses_3xx"),
		status4xx: getOrNewInt("octarq_responses_4xx"),
		status5xx: getOrNewInt("octarq_responses_5xx"),
		inFlight:  getOrNewInt("octarq_requests_in_flight"),
	}
}

func getOrNewInt(name string) *expvar.Int {
	if v := expvar.Get(name); v != nil {
		if iv, ok := v.(*expvar.Int); ok {
			return iv
		}
	}
	return expvar.NewInt(name)
}

// record updates the counters for one completed request.
func (m *metrics) record(status int) {
	m.requests.Add(1)
	switch {
	case status >= 500:
		m.status5xx.Add(1)
	case status >= 400:
		m.status4xx.Add(1)
	case status >= 300:
		m.status3xx.Add(1)
	default:
		m.status2xx.Add(1)
	}
}

// snapshot renders the current metrics as a JSON-serialisable map.
func (m *metrics) snapshot() map[string]any {
	return map[string]any{
		"uptime_seconds":     int64(time.Since(m.startedAt).Seconds()),
		"requests_total":     m.requests.Value(),
		"responses_2xx":      m.status2xx.Value(),
		"responses_3xx":      m.status3xx.Value(),
		"responses_4xx":      m.status4xx.Value(),
		"responses_5xx":      m.status5xx.Value(),
		"requests_in_flight": m.inFlight.Value(),
	}
}
