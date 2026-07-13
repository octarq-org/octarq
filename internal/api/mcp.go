package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/internal/auth"
	mcp_internal "github.com/octarq-org/octarq/internal/mcp"
	"github.com/octarq-org/octarq/internal/models"
)

// mcpAuth is a middleware that authenticates MCP requests. It checks the session cookie,
// the Authorization Bearer header, and falls back to the ?token= query parameter.
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
			if h.db.Where("hash = ?", hash).First(&tok).Error == nil && !tok.Expired() {
				ctx := auth.WithOrgID(r.Context(), tok.OrgID)
				id := tok.ID
				db := h.db
				go func() {
					now := time.Now()
					db.Model(&models.Token{}).Where("id = ?", id).Update("last_used_at", &now)
				}()
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		writeErr(w, http.StatusUnauthorized, "unauthorized: missing or invalid token")
	})
}

// mcpSSEHandler returns an http.Handler that handles MCP over SSE (Server-Sent Events).
func (h *Handler) mcpSSEHandler() http.Handler {
	handler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		orgID := h.orgID(r)
		// allowRawSQL=false: over HTTP the caller is one tenant among many, and raw
		// SQL can't be scoped to a single owner_id. Only the tenant-scoped tools run.
		return mcp_internal.NewServerInstance(h.db, orgID, h.plugins, false)
	}, &mcp.SSEOptions{
		DisableLocalhostProtection: true,
	})
	return h.mcpAuth(handler)
}

// mcpStreamHandler returns an http.Handler that handles MCP over Streamable HTTP.
func (h *Handler) mcpStreamHandler() http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		orgID := h.orgID(r)
		// allowRawSQL=false: over HTTP the caller is one tenant among many, and raw
		// SQL can't be scoped to a single owner_id. Only the tenant-scoped tools run.
		return mcp_internal.NewServerInstance(h.db, orgID, h.plugins, false)
	}, &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
	})
	return h.mcpAuth(handler)
}
