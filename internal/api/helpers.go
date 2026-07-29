package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/authz"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

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

// requireRole rejects a caller whose workspace role is below min.
func (h *Handler) requireRole(r *http.Request, min authz.Role) error {
	if h.callerHoldsRole(r, min) {
		return nil
	}
	return huma.Error403Forbidden("forbidden: insufficient workspace role privilege")
}

// callerHoldsRole is the single place both role gates agree on — core's
// requireRole and, through plugin.Context.RequireRole, every Pro gate.
//
// An API bearer token authenticates as the workspace, not as a person: the auth
// layer deliberately leaves the user id at 0, so callerOrgRole finds no
// membership row and would return "" for every token request. Comparing against
// that would refuse every token — CI jobs, scripts, integrations.
//
// So a token is judged on the role it was minted with (P2-18) rather than on a
// membership it does not have. A token with no role is one minted before scoping
// existed: it keeps the unrestricted access it has always had, because
// retroactively narrowing tokens already deployed in someone's CI would break
// them silently, at a time nobody is watching. Those tokens are flagged as
// unrestricted in the dashboard so an operator can rotate them deliberately.
func (h *Handler) callerHoldsRole(r *http.Request, min authz.Role) bool {
	if auth.TokenIDFromContext(r.Context()) != 0 {
		role := auth.TokenRoleFromContext(r.Context())
		if role == "" {
			return true // legacy token, minted before P2-18
		}
		return authz.AtLeast(authz.Role(role), min)
	}
	return authz.AtLeast(authz.Role(h.callerOrgRole(r)), min)
}

// RequireRole reports whether the caller holds at least min role in their active org.
// Exposed to plugins via plugin.Context.RequireRole.
func (h *Handler) RequireRole(r *http.Request, min string) bool {
	return h.callerHoldsRole(r, authz.Role(min))
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
