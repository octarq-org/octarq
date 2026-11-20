package api

import (
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/auth"
	mcp_internal "github.com/octarq-org/octarq/server/internal/mcp"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
)

func (h *Handler) mcpAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.auth.OrgID(r) != 0 {
			next.ServeHTTP(w, r)
			return
		}

		token := r.URL.Query().Get("token")
		if strings.HasPrefix(token, "oct_") {
			hash := models.HashToken(token)
			var tok models.Token
			if h.db.Where("hash = ?", hash).First(&tok).Error == nil && !tok.Expired() && tok.OrgID != 0 {
				ctx := auth.WithOrgID(r.Context(), tok.OrgID)
				ctx = auth.WithUserID(ctx, tok.UserID)
				ctx = h.auth.WithTokenIdentity(ctx, tok)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		writeErr(w, r, http.StatusUnauthorized, "", "Authentication required; provide a valid token")
	})
}

func (h *Handler) mcpSSEHandler() http.Handler {
	handler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		orgID := h.orgID(r)
		return mcp_internal.NewNetworkedServerInstance(h.db, orgID, h.activePlugins(orgID), h.LookupService)
	}, &mcp.SSEOptions{
		DisableLocalhostProtection: true,
	})
	authed := h.mcpAuth(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		telemetry.IncMCPSessions()
		defer telemetry.DecMCPSessions()
		authed.ServeHTTP(w, r)
	})
}

func (h *Handler) mcpStreamHandler() http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		orgID := h.orgID(r)
		return mcp_internal.NewNetworkedServerInstance(h.db, orgID, h.activePlugins(orgID), h.LookupService)
	}, &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
	})
	authed := h.mcpAuth(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		telemetry.IncMCPSessions()
		defer telemetry.DecMCPSessions()
		authed.ServeHTTP(w, r)
	})
}
