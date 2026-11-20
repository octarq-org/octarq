package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCharacterization_PublicVsProtectedEndpoints(t *testing.T) {
	_, handler, _ := newTestHandlerRaw(t)

	reqHealth := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)
	require.Equal(t, http.StatusOK, recHealth.Code, "public health check must return 200")

	protectedPaths := []string{
		"/api/auth/me",
		"/api/overview",
		"/api/settings",
		"/api/tokens",
		"/api/notifications",
		"/api/notification-channels",
		"/api/webhooks",
		"/api/orgs",
	}

	for _, path := range protectedPaths {
		t.Run("Protected_"+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code, "path %s must reject unauthenticated requests with 401", path)

			var env map[string]any
			err := json.Unmarshal(rec.Body.Bytes(), &env)
			require.NoError(t, err, "response body must parse as valid json envelope")
			require.Equal(t, "unauthorized", env["code"], "error envelope must contain code: unauthorized")
			require.NotEmpty(t, env["message"], "message field should be present in error")
		})
	}
}

func TestCharacterization_RateLimiterBudgetIsolation(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)
	require.NotNil(t, h, "handler must initialize correctly")

	authRPM, apiRPM, redirectRPM := h.RateLimits()
	require.Equal(t, 60, authRPM, "default auth RPM must be 60")
	require.Equal(t, 600, apiRPM, "default api RPM must be 600")
	require.Equal(t, 6000, redirectRPM, "default redirect RPM must be 6000")
}

func TestCharacterization_HumaAPIRegistrationIntegrity(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)
	apiInstance := h.Huma()
	require.NotNil(t, apiInstance, "Huma API instance must be registered on Handler")

	openAPI := apiInstance.OpenAPI()
	require.NotNil(t, openAPI, "OpenAPI model must not be nil")
	require.Equal(t, "Octarq API", openAPI.Info.Title)
	require.Equal(t, "1.0.0", openAPI.Info.Version)

	paths := openAPI.Paths
	require.NotNil(t, paths["/api/health"], "path /api/health must be registered")
	require.NotNil(t, paths["/api/auth/login"], "path /api/auth/login must be registered")
	require.NotNil(t, paths["/api/auth/me"], "path /api/auth/me must be registered")
	require.NotNil(t, paths["/api/tokens"], "path /api/tokens must be registered")
	require.NotNil(t, paths["/api/settings"], "path /api/settings must be registered")
}

func TestCharacterization_ZeroOrgIDFailClosed(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)

	val := h.GetWorkspaceSetting(0, "some_key")
	require.Empty(t, val, "orgID=0 must return empty setting")

	err := h.SetWorkspaceSetting(0, "some_key", "value")
	require.Error(t, err, "orgID=0 must fail closed with error")
}
