package api

import (
	"net/http"
	"net/url"
	"strings"
)

// sessionCookieName is the browser session cookie (mirrors auth.cookieName).
// Its presence marks a request as browser-driven and therefore CSRF-relevant.
const sessionCookieName = "led_session"

// CSRFGuard wraps an API handler and blocks cross-site state-changing requests
// that ride on the ambient session cookie. Bearer-token / webhook-token clients
// (which set no session cookie) are never gated — they aren't a CSRF vector,
// since an attacker page can't read or forge those headers.
//
// The check is Origin/Referer-based: a forged cross-site form or fetch always
// carries an Origin (or at least a Referer) that names the attacker's site, so a
// mismatch against the request Host is refused. Same-origin app requests pass.
// This complements the cookie's SameSite=Lax attribute (which Lax leaves a small
// top-level-navigation gap for).
func CSRFGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutating(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := r.Cookie(sessionCookieName); err != nil {
			// No session cookie → not a browser CSRF vector (bearer/webhook auth).
			next.ServeHTTP(w, r)
			return
		}
		if !sameOriginRequest(r) {
			writeErr(w, http.StatusForbidden, "cross-origin request blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// sameOriginRequest reports whether the request's Origin (preferred) or Referer
// names the same host it was sent to. When neither header is present the request
// is allowed: browsers attach Origin to cross-site state-changing requests, so
// absence means a non-browser client, not a CSRF attempt.
func sameOriginRequest(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return originHostMatches(origin, r.Host)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		return originHostMatches(ref, r.Host)
	}
	return true
}

func originHostMatches(rawURL, host string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
