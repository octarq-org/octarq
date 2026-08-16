package api

import (
	"strings"
	"testing"
)

// registeredPublicEndpoints is the reviewed inventory of routes reachable
// without a dashboard session, together with how each one authenticates its
// caller instead. Every entry was checked against its handler; the reasoning
// is written up in the AUDIT-public-endpoints audit (kept outside this repo).
//
// THIS LIST IS A REVIEW GATE, NOT A CONFIG. Adding a line here makes a route
// publicly reachable in the eyes of this test, so a diff that grows it is a
// diff that needs a security read. Do not paste in whatever the failure
// message printed — that defeats the only purpose the list has.
var registeredPublicEndpoints = map[string]string{
	// Sign-in and account recovery. These cannot require a session by
	// definition; each is rate-limited by the middleware's auth tier and the
	// token-bearing ones (invite / reset / verify) validate a single-use
	// hashed token before doing anything.
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

	// Liveness. No tenant data, no side effects.
	"GET /api/health": "liveness probe",
	"GET /api/status": "liveness probe",

	// Abuse reporting. Registered outside /api/, so this gate never saw it at
	// all. Public by design — a takedown request comes from a stranger — but it
	// writes a row, so it is rate-limited as its own tier in
	// internal/server/middleware.go:205.
	"POST /abuse": "public by design; rate-limited as its own tier",

	// DDNS. A router calls this on a schedule with no cookie; the caller is
	// identified by a hashed per-record token (plugins/dns/ddns.go), which is
	// the whole authentication for the route.
	"GET /api/dns/ddns/update":  "hashed per-record DDNS token",
	"POST /api/dns/ddns/update": "hashed per-record DDNS token",

	// Inbound mail and bounce notifications. These arrive from Cloudflare Email
	// Routing, SendGrid/Mailgun and SES/SNS with no session, and often from a
	// provider whose only configurable field is a URL. Each carries the org's
	// inbound token as a path segment, constant-time compared; it rotates from
	// Mail settings. The SNS path additionally validates the SubscribeURL host
	// so a confirmation cannot point the server at an internal address
	// (isAWSSNSURL in plugins/mail/mail.go).
	"POST /api/webhook/{orgSlug}/email/inbound/{token}":     "org inbound token in path, constant-time compared",
	"POST /api/webhook/{orgSlug}/email/inbound/raw/{token}": "org inbound token in path, constant-time compared",
	"POST /api/webhook/{orgSlug}/email/bounce/{token}":      "org inbound token in path; SNS SubscribeURL host validated",
}

// TestPublicEndpointRegistry fails when the set of routes reachable without a
// session stops matching the reviewed inventory above.
//
// Two mechanisms can make a route public — Metadata["public"] on the operation,
// and a prefix in publicPathPrefixes — and neither leaves a trace anywhere a
// reviewer would look. A plugin author adding Metadata["public"] to the wrong
// operation, or mounting a route under the /api/webhook/ subtree, publishes an
// unauthenticated endpoint and nothing says so. This test says so.
//
// When it fails: do not edit the map to match. Open the handler, work out
// whether it authenticates its own caller, and only then write the line.
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

// TestPublicPrefixesAreNarrow pins the blast radius of subtree exemptions.
//
// Exact paths exempt one endpoint each. A subtree exempts everything beneath it,
// including routes that do not exist yet, so each one is a standing invitation
// to publish something by accident. There is exactly one, it is /api/webhook/,
// and it has to be a subtree because webhook URLs carry the workspace slug and
// a per-mailbox token as path segments.
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

// TestLogoutAllIsNotExemptByPrefix pins the bug that motivated exact matching.
//
// The old gate matched "/api/auth/logout" as a prefix, which also matched
// "/api/auth/logout-all" — an endpoint that deletes every session a user has.
// It was never exploitable, because logoutAll re-authenticates on its own
// (auth.go:189), but the gate was not the reason. Reintroduce prefix matching
// and this goes red.
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
		// Near-misses: these share a prefix with a public route up to a point
		// and must still be gated.
		"/api/authx/login",
		"/api/webhooks",
		// Build info fingerprints the instance's version for CVE scanning.
		"/api/instance/build",
	} {
		if isPublicPath(path) {
			t.Errorf("%q must require a session", path)
		}
	}
}

// TestPublicGETMatcher pins what CORS is allowed to attach to: public GET
// endpoints and only those. A non-public GET, or a public route that is not a
// GET (login is public but mutating), must never qualify — CORS is a read-only
// concession and the whitelist must not widen it into a write path.
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
		"/api/auth/login", // public, but POST — never a CORS read
		"/api/auth/logout",
	} {
		if publicGET(path) {
			t.Errorf("%s must NOT qualify for CORS", path)
		}
	}
}
