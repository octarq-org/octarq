package server

import (
	"net/http"
	"strings"
)

// originAllowed reports whether origin is an EXACT member of allow. Prefix and
// substring matches are deliberately rejected: "https://octarq.org.evil.com"
// must never pass because it merely contains "octarq.org".
func originAllowed(origin string, allow []string) bool {
	for _, a := range allow {
		if origin == a {
			return true
		}
	}
	return false
}

// applyCORS writes cross-origin headers for requests to public GET endpoints
// whose Origin is in the configured allowlist, and short-circuits an approved
// preflight. It returns true only when the request was a preflight that this
// middleware already answered (204) and the router must not see.
//
// The security contract is deliberate and non-negotiable:
//   - only GET endpoints the auth gate lets through unauthenticated;
//   - the Origin must match the allowlist exactly;
//   - Access-Control-Allow-Credentials is never set. These endpoints are public
//     data; once credentials are allowed, any whitelisted origin that is ever
//     compromised can act as the user against every API on the instance.
func (mw *middleware) applyCORS(w http.ResponseWriter, r *http.Request) bool {
	if mw.settings.PublicGET == nil {
		return false
	}
	if !mw.settings.PublicGET(r.URL.Path) {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin header: same-origin or non-browser client, no CORS needed.
		return false
	}

	if r.Method == http.MethodOptions {
		// Preflight. Only a GET read is ever on offer; a preflight asking for
		// anything else is not answered (the browser will block the follow-up).
		if !strings.EqualFold(r.Header.Get("Access-Control-Request-Method"), http.MethodGet) {
			return false
		}
		if !originAllowed(origin, mw.currentCORSOrigins()) {
			return false
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Methods", http.MethodGet)
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			h.Set("Access-Control-Allow-Headers", reqHeaders)
		}
		h.Set("Access-Control-Max-Age", "600")
		h.Add("Vary", "Origin")
		h.Add("Vary", "Access-Control-Request-Method")
		h.Add("Vary", "Access-Control-Request-Headers")
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	if r.Method != http.MethodGet {
		return false
	}
	// Vary on Origin whenever the response is origin-dependent, so a shared
	// cache never serves a non-CORS copy to an allowed origin or vice versa.
	if originAllowed(origin, mw.currentCORSOrigins()) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Add("Vary", "Origin")
	return false
}

// contentSecurityPolicy is the baseline CSP applied to every response.
//
// script-src deliberately has NO 'unsafe-inline': without it, an XSS that finds
// a single injection point cannot execute arbitrary inline scripts — CSP is a
// real mitigation layer. The two inline scripts the dashboard genuinely ships
// (the synchronous theme toggle and the campaign-forwarding snippet in
// index.html) are allow-listed by their exact build-time SHA-256 hashes instead.
// Rebuilding index.html changes those hashes; the hash values here must be
// regenerated from the served index.html (see setSecurityHeaders' guard test,
// which recomputes them from the embedded dist and fails if they drift).
//
// style-src KEEPS 'unsafe-inline': Tailwind injects its styles at runtime and
// removing it would blank the whole dashboard.
const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'sha256-XOdMZOShyEmv5lOxX1JVnl4Ve1llBlA5kNEaund+AXQ=' 'sha256-0la+svl4yiuBleyZsFu+6aWjFtuvmwtPoqXKtkLO42Q='; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'"

// setSecurityHeaders applies baseline hardening headers to every response.
func setSecurityHeaders(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	}
}
