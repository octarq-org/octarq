package api

import (
	"crypto/hmac"
	"net/http"
	"net/url"
	"strings"

	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/csrf"
	"github.com/octarq-org/octarq/server/origin"
)

const sessionCookieName = "octarq_session"

func CSRFGuard(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Cookies()) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		if !isMutating(r.Method) {
			if sc, err := r.Cookie(sessionCookieName); err == nil && !hasValidToken(r, secret, sc.Value) {
				http.SetCookie(w, csrf.NewCookie(secret, sc.Value, origin.Secure(r, trustProxy.Load())))
			}
			next.ServeHTTP(w, r)
			return
		}

		if !sameOriginRequest(r) {
			writeErr(w, r, http.StatusForbidden, apierror.CodeCSRFOriginBlocked, "Cross-origin request blocked; make requests from the same origin")
			return
		}

		if sc, err := r.Cookie(sessionCookieName); err == nil {
			if !hmac.Equal([]byte(r.Header.Get(csrf.HeaderName)), []byte(csrf.GenerateToken(secret, sc.Value))) {
				writeErr(w, r, http.StatusForbidden, apierror.CodeCSRFTokenInvalid, "CSRF token is invalid or missing; reload the page and try again")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

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
