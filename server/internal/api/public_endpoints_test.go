package api

import (
	"strings"
	"testing"
)

var registeredPublicEndpoints = map[string]string{
	"POST /api/auth/login":               "credentials + optional TOTP; auth-tier rate limited",
	"POST /api/auth/register":            "gated by the instance's registration setting",
	"POST /api/auth/2fa/verify":          "consumes a pending 2FA challenge",
	"POST /api/auth/logout":              "clears the caller's own cookie",
	"GET /api/auth/config":               "public instance config; no tenant data",
	"GET /api/auth/methods":              "which login methods are enabled; no tenant data",
	"POST /api/auth/invite/accept":       "single-use hashed invite token",
	"POST /api/auth/forgot":              "always answers 200 so it cannot enumerate accounts",
	"POST /api/auth/reset":               "single-use hashed reset token with expiry",
	"GET /api/auth/verify-email":         "single-use hashed verification token",
	"POST /api/auth/resend-verification": "always answers 200 so it cannot enumerate accounts",

	"GET /api/health": "liveness probe",
	"GET /api/status": "liveness probe",

	"POST /abuse": "public by design; rate-limited as its own tier",

	"GET /api/files/{id}/download": "session or HMAC-SHA256 signed temporary download token",

	"GET /api/account/export": "session (owner/admin) or HMAC-SHA256 signed temporary download token",
}

func TestPublicEndpointRegistry(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)

	var found []string
	seen := map[string]bool{}
	for _, ep := range PublicOperations(h.Huma()) {
		key := ep.Method + " " + ep.Path
		seen[key] = true
		if _, ok := registeredPublicEndpoints[key]; !ok {
			found = append(found, key+"  ["+ep.Reason+"]")
		}
	}

	if len(found) > 0 {
		t.Errorf("unregistered public endpoint(s) — these are reachable with NO session:\n  %s\n\n"+
			"If each one authenticates its own caller (webhook signature, single-use token,\n"+
			"buyer session), add it to registeredPublicEndpoints with that reason. If it does\n"+
			"not, it is an authentication bypass — fix the route, not this test.",
			strings.Join(found, "\n  "))
	}

	for key := range registeredPublicEndpoints {
		if !seen[key] {
			t.Errorf("registered public endpoint %q is no longer public (or no longer exists) — "+
				"drop it from registeredPublicEndpoints so the list keeps meaning something", key)
		}
	}
}

func TestPublicPrefixesAreNarrow(t *testing.T) {
	if len(publicSubtreePrefixes) != 1 || publicSubtreePrefixes[0] != "/api/webhook/" {
		t.Errorf("expected exactly one subtree exemption (/api/webhook/), got %v.\n"+
			"A subtree exempts every route under it, including ones nobody has written yet.\n"+
			"Prefer an exact path so each new public route is a visible change.", publicSubtreePrefixes)
	}
	for p := range publicExactPaths {
		if strings.HasSuffix(p, "/") {
			t.Errorf("publicExactPaths entry %q ends in a slash — it reads as a subtree but is "+
				"matched exactly, which will confuse the next reader. Drop the slash or move it "+
				"to publicSubtreePrefixes deliberately.", p)
		}
	}
}

func TestLogoutAllIsNotExemptByPrefix(t *testing.T) {
	if isPublicPath("/api/auth/logout-all") {
		t.Error("/api/auth/logout-all must require a session — it deletes every session " +
			"the user has. It was previously exempt only because it shares a prefix with " +
			"/api/auth/logout.")
	}
}

func TestIsPublicPathRequiresSessionByDefault(t *testing.T) {
	for _, path := range []string{
		"/api/links",
		"/api/mailboxes",
		"/api/auth/me",
		"/api/auth/password",
		"/api/authx/login",
		"/api/webhooks",
		"/api/instance/build",
	} {
		if isPublicPath(path) {
			t.Errorf("%q must require a session", path)
		}
	}
}

func TestPublicGETMatcher(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)
	publicGET := PublicGETMatcher(h.Huma())

	for _, path := range []string{"/api/auth/config", "/api/auth/methods", "/api/health", "/api/status"} {
		if !publicGET(path) {
			t.Errorf("public GET path %s must qualify for CORS", path)
		}
	}

	for _, path := range []string{
		"/api/auth/me",
		"/api/links",
		"/api/mailboxes",
		"/api/auth/login",
		"/api/auth/logout",
	} {
		if publicGET(path) {
			t.Errorf("%s must NOT qualify for CORS", path)
		}
	}
}
