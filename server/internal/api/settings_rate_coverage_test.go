package api

import "testing"

func TestRateLimitsReadEnvDefaults(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)

	t.Setenv("OCTARQ_RATELIMIT_AUTH_RPM", "not-a-number")
	t.Setenv("OCTARQ_RATELIMIT_API_RPM", "not-a-number")
	if got, got2, _ := h.RateLimits(); got != defaultAuthRPM || got2 != defaultAPIRPM {
		t.Errorf("invalid env RateLimits() = (%d, %d)", got, got2)
	}

	t.Setenv("OCTARQ_RATELIMIT_AUTH_RPM", "77")
	t.Setenv("OCTARQ_RATELIMIT_API_RPM", "88")
	if got, got2, _ := h.RateLimits(); got != 77 || got2 != 88 {
		t.Errorf("env RateLimits() = (%d, %d), want (77, 88)", got, got2)
	}
}
