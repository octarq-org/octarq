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
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// trustProxy gates whether proxy-supplied client-IP headers are honoured. Set
// once from config in New; when false, a client cannot spoof X-Forwarded-For
// to get a fresh abuse-report rate-limit bucket.
var trustProxy bool

// reporterIP returns the best-guess client IP for rate limiting and abuse reports.
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
	import_net_SplitHostPort := func(hostport string) (string, string, error) {
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

// effectiveRole is the authority a request actually carries, and every
// authorization decision must go through it rather than through callerOrgRole.
//
// A bearer token acts as the person who minted it, so the ordinary membership
// lookup answers for both credentials — one authorization path, not a parallel
// one only tokens take. The membership is read live, which is what makes
// removing someone from the workspace revoke their tokens with them.
//
// The token's own role then narrows that, and only narrows it: the effective
// role is min(the holder's membership, the token's cap). Reading callerOrgRole
// directly skips the cap, which is how a member-capped token briefly became
// able to grant workspace ownership — the holder was an owner, and the check
// never asked what the token was allowed to do.
//
// An empty cap reads as "member", the same thing minting defaults to, so a row
// that somehow carries no role is the least privileged rather than the most.
func (h *Handler) effectiveRole(r *http.Request) authz.Role {
	role := authz.Role(h.callerOrgRole(r))
	if auth.TokenIDFromContext(r.Context()) == 0 {
		return role
	}
	cap := authz.Role(auth.TokenRoleFromContext(r.Context()))
	if cap == "" {
		cap = authz.RoleMember
	}
	if authz.AtLeast(role, cap) {
		return cap
	}
	return role
}

// callerHoldsRole is the single place both role gates agree on — core's
// requireRole and, through plugin.Context.RequireRole, every Pro gate.
func (h *Handler) callerHoldsRole(r *http.Request, min authz.Role) bool {
	return authz.AtLeast(h.effectiveRole(r), min)
}

// RequireRole reports whether the caller holds at least min role in their active org.
// Exposed to plugins via plugin.Context.RequireRole.
func (h *Handler) RequireRole(r *http.Request, min string) bool {
	return h.callerHoldsRole(r, authz.Role(min))
}

// RequirePerm reports whether the caller holds permKey, falling back to the
// built-in role comparison when no resolver has an opinion.
// Exposed to plugins via plugin.Context.RequirePerm.
func (h *Handler) RequirePerm(r *http.Request, permKey, minRole string) bool {
	if r == nil {
		return false
	}
	// A bearer token carries a role cap, and the cap is a ceiling no resolver may
	// cross: a resolver is only allowed to refine permissions above the role
	// baseline, never to hand a capped token more than its own role permits. So
	// token requests must clear minRole on their own first — through
	// callerHoldsRole, which is AtLeast(effectiveRole) and therefore already
	// clamps to the cap — and are refused outright otherwise. Sessions have no
	// cap, so this pre-gate is token-only: a plugin-granted custom role may still
	// widen a session's authority above its base role, and that is the Pro
	// behaviour this must not break.
	if auth.TokenIDFromContext(r.Context()) != 0 {
		if h == nil {
			return false
		}
		if !h.callerHoldsRole(r, authz.Role(minRole)) {
			return false
		}
	}
	if allow, decided := plugin.ResolvePerm(r, permKey); decided {
		return allow
	}
	if h == nil {
		return false
	}
	return h.RequireRole(r, minRole)
}

// auditAs writes an AuditLog entry with explicit attribution asynchronously;
// never blocks a request. Used where the actor or org is known outside the
// request's session — login/register run before a session exists, so deriving
// them from the request (h.orgID / h.auth.UserID) would record 0/0.
// meta is an optional map that is JSON-encoded (pass nil to omit).
func (h *Handler) auditAs(r *http.Request, orgID, actorID uint, action, targetType string, targetID uint, meta map[string]any) {
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

// audit writes an AuditLog entry attributed to the request's session,
// asynchronously; never blocks a request. Thin wrapper around auditAs — call
// auditAs directly when the actor or org is known outside the session.
func (h *Handler) audit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	h.auditAs(r, h.orgID(r), h.auth.UserID(r), action, targetType, targetID, meta)
}

// Audit is a public wrapper around audit, exposed for plugins via plugin.Context.
func (h *Handler) Audit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	h.audit(r, action, targetType, targetID, meta)
}

// Domain/DNS provider helpers (encryptConfig, providerFor) moved to the dns
// Core plugin (plugins/dns). See website/src/content/docs/architecture/overview.md.
