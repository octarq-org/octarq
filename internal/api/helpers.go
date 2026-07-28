package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

var errNotFound = errors.New("not found")

// secureEqual reports whether a and b are equal using a constant-time
// comparison, avoiding timing side channels when checking secret tokens.
func secureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// trustProxy gates whether proxy-supplied client-IP headers are honoured. Set
// once from config in New; when false, a client cannot spoof X-Forwarded-For
// to get a fresh abuse-report rate-limit bucket.
var trustProxy bool

// reporterIP returns the best-guess client IP for abuse reports.
// We keep the full IP here (unlike analytics) so admins can block repeat abusers.
func reporterIP(r *http.Request) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
		if rip := r.Header.Get("X-Real-IP"); rip != "" {
			return rip
		}
	}
	host := r.RemoteAddr
	if h, _, err := splitHostPort(host); err == nil {
		return h
	}
	return host
}

func splitHostPort(addr string) (host, port string, err error) {
	// Thin wrapper so abuse.go doesn't import "net" directly.
	import_net_SplitHostPort := func(hostport string) (string, string, error) {
		// inline net.SplitHostPort to keep imports clean
		for i := len(hostport) - 1; i >= 0; i-- {
			if hostport[i] == ':' {
				return hostport[:i], hostport[i+1:], nil
			}
		}
		return "", "", errors.New("no port")
	}
	return import_net_SplitHostPort(addr)
}

// orgID returns the authenticated org, or 0 when the request carries none.
// Zero is NOT substituted with a default tenant: on a multi-tenant host that
// would serve the bootstrap org's data to an unidentified caller.
func (h *Handler) orgID(r *http.Request) uint {
	return h.auth.OrgID(r)
}

// orgDB returns a *gorm.DB pre-scoped to the authenticated org.
// Use instead of h.db for any query that should be tenant-isolated.
// Returns a guaranteed empty query (1 = 0) if org is 0 as defense-in-depth.
func (h *Handler) orgDB(r *http.Request) *gorm.DB {
	id := h.orgID(r)
	if id == 0 {
		return h.db.Where("1 = 0")
	}
	return h.db.Where("owner_id = ?", id)
}

// requireOrg resolves the caller's org or reports that the request cannot be
// served. Handlers that read or write tenant data must use this rather than
// orgID, so an unidentified caller gets 401 instead of someone else's data.
func (h *Handler) requireOrg(r *http.Request) (uint, error) {
	id := h.auth.OrgID(r)
	if id == 0 {
		return 0, huma.Error401Unauthorized("unauthorized: missing workspace context")
	}
	return id, nil
}

// audit writes an AuditLog entry asynchronously; never blocks a request.
// meta is an optional map that is JSON-encoded (pass nil to omit).
func (h *Handler) audit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	orgID := h.orgID(r)
	actorID := h.auth.UserID(r)
	ip := reporterIP(r)
	if tokID := auth.TokenIDFromContext(r.Context()); tokID != 0 {
		if meta == nil {
			meta = make(map[string]any)
		}
		meta["tokenId"] = tokID
	}
	var metaJSON string
	if meta != nil {
		if b, err := json.Marshal(meta); err == nil {
			metaJSON = string(b)
		}
	}
	go func() {
		h.db.Create(&models.AuditLog{
			OrgID:      orgID,
			ActorID:    actorID,
			Action:     action,
			TargetType: targetType,
			TargetID:   targetID,
			Meta:       metaJSON,
			IP:         ip,
		})
	}()
}

// Audit is a public wrapper around audit, exposed for plugins via plugin.Context.
func (h *Handler) Audit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	h.audit(r, action, targetType, targetID, meta)
}

// Domain/DNS provider helpers (encryptConfig, providerFor) moved to the dns
// Core plugin (plugins/dns). See docs/CORE-PLUGIN-EXTRACTION.md.
