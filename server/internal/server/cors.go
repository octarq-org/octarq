package server

import (
	"net/http"
	"strings"
)

func originAllowed(origin string, allow []string) bool {
	for _, a := range allow {
		if origin == a {
			return true
		}
	}
	return false
}

func (mw *middleware) applyCORS(w http.ResponseWriter, r *http.Request) bool {
	if mw.settings.PublicGET == nil {
		return false
	}
	if !mw.settings.PublicGET(r.URL.Path) {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}

	if r.Method == http.MethodOptions {
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
	if originAllowed(origin, mw.currentCORSOrigins()) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Add("Vary", "Origin")
	return false
}

const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval' 'sha256-XOdMZOShyEmv5lOxX1JVnl4Ve1llBlA5kNEaund+AXQ=' 'sha256-0la+svl4yiuBleyZsFu+6aWjFtuvmwtPoqXKtkLO42Q='; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'"

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
