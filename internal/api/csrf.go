package api

import (
	"crypto/hmac"
	"net/http"
	"net/url"
	"strings"

	"github.com/octarq-org/octarq/internal/apierror"
	"github.com/octarq-org/octarq/internal/csrf"
	"github.com/octarq-org/octarq/origin"
)

// sessionCookieName is the dashboard session cookie (mirrors auth.cookieName).
// It selects which requests must carry the double-submit token — NOT which
// requests are browser-driven. Those are two different questions, and conflating
// them is what left a hole; see CSRFGuard.
const sessionCookieName = "octarq_session"

// CSRFGuard wraps an API handler and blocks cross-site state-changing requests
// that ride on an ambient cookie. It applies two independent checks, gated on
// two different conditions — read them separately.
//
// # Origin/Referer — gated on the request carrying ANY cookie
//
// A forged cross-site form or fetch always carries an Origin (or at least a
// Referer) naming the attacker's site, so a mismatch against the request Host is
// refused. Same-origin app requests pass. This complements the cookie's
// SameSite=Lax attribute, which leaves a small top-level-navigation gap.
//
// The trigger is "any cookie present", not "the session cookie present". octarq
// has more than one browser session: the Pro buyer portal authenticates with its
// own octarq_customer cookie, and a plugin may add others. Keying off one name
// left every one of those protected by SameSite alone. Any cookie at all is the
// right question, because it is a superset that costs nothing: a bearer-token or
// webhook client sends no cookies and is waved through above, and a non-browser
// client that does send cookies sends neither Origin nor Referer, which
// sameOriginRequest already allows.
//
// # Double-submit token — gated on the DASHBOARD session cookie
//
// Requests bearing octarq_session must also echo the derived token (see package
// internal/csrf) in the X-CSRF-Token header. This check deliberately does NOT
// extend to other sessions: nothing mints a token for octarq_customer, so
// demanding one would lock every buyer out of the portal. Those sessions are
// covered by the Origin/Referer check above.
func CSRFGuard(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Cookies()) == 0 {
			// No cookies at all → not a browser CSRF vector (bearer/webhook auth).
			next.ServeHTTP(w, r)
			return
		}

		if !isMutating(r.Method) {
			// Sessions minted before this guard existed have no token cookie, and
			// their owners would be locked out of every write until they logged in
			// again. Re-issue it on safe methods instead — the SPA always GETs
			// before it writes. Never on a write: that would hand the token to the
			// very request being refused.
			if sc, err := r.Cookie(sessionCookieName); err == nil && !hasValidToken(r, secret, sc.Value) {
				http.SetCookie(w, csrf.NewCookie(secret, sc.Value, origin.Secure(r, trustProxy)))
			}
			next.ServeHTTP(w, r)
			return
		}

		if !sameOriginRequest(r) {
			writeErr(w, r, http.StatusForbidden, apierror.CodeCSRFOriginBlocked, "cross-origin request blocked")
			return
		}

		if sc, err := r.Cookie(sessionCookieName); err == nil {
			if !hmac.Equal([]byte(r.Header.Get(csrf.HeaderName)), []byte(csrf.GenerateToken(secret, sc.Value))) {
				writeErr(w, r, http.StatusForbidden, apierror.CodeCSRFTokenInvalid, "invalid csrf token")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// hasValidToken reports whether the request already carries the token cookie
// matching sessionToken, i.e. whether the re-issue above can be skipped.
func hasValidToken(r *http.Request, secret, sessionToken string) bool {
	c, err := r.Cookie(csrf.CookieName)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(c.Value), []byte(csrf.GenerateToken(secret, sessionToken)))
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
