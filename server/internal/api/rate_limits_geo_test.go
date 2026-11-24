package api

import "testing"

func TestRateLimitsEnvGeoSlice(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)

	t.Setenv("OCTARQ_RATELIMIT_AUTH_RPM", "invalid")
	t.Setenv("OCTARQ_RATELIMIT_API_RPM", "invalid")
	if auth, api, _ := h.RateLimits(); auth != defaultAuthRPM || api != defaultAPIRPM {
		t.Errorf("invalid env = (%d, %d)", auth, api)
	}

	t.Setenv("OCTARQ_RATELIMIT_AUTH_RPM", "77")
	t.Setenv("OCTARQ_RATELIMIT_API_RPM", "88")
	if auth, api, _ := h.RateLimits(); auth != 77 || api != 88 {
		t.Errorf("env = (%d, %d), want (77, 88)", auth, api)
	}
}
