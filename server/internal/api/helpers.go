package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

var trustProxy atomic.Bool

func reporterIP(r *http.Request) string {
	if trustProxy.Load() {
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

func (h *Handler) orgID(r *http.Request) uint {
	return h.auth.OrgID(r)
}

func (h *Handler) orgDB(r *http.Request) *gorm.DB {
	id := h.orgID(r)
	if id == 0 {
		return h.db.Where("1 = 0")
	}
	return h.db.Where("owner_id = ?", id)
}

func (h *Handler) requireOrg(r *http.Request) (uint, error) {
	id := h.auth.OrgID(r)
	if id == 0 {
		return 0, huma.Error401Unauthorized("Workspace context is missing; select a workspace to continue")
	}
	return id, nil
}

func (h *Handler) requireRole(r *http.Request, min authz.Role) error {
	if h.callerHoldsRole(r, min) {
		return nil
	}
	return huma.Error403Forbidden("Workspace permission denied; higher workspace role required")
}

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

func (h *Handler) callerHoldsRole(r *http.Request, min authz.Role) bool {
	return authz.AtLeast(h.effectiveRole(r), min)
}

func (h *Handler) RequireRole(r *http.Request, min string) bool {
	return h.callerHoldsRole(r, authz.Role(min))
}

func (h *Handler) RequirePerm(r *http.Request, permKey, minRole string) bool {
	if r == nil {
		return false
	}
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

func (h *Handler) audit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	h.auditAs(r, h.orgID(r), h.auth.UserID(r), action, targetType, targetID, meta)
}

func (h *Handler) Audit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	h.audit(r, action, targetType, targetID, meta)
}
