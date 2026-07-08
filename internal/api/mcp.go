package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/led/internal/auth"
	mcp_internal "github.com/octarq-org/led/internal/mcp"
	"github.com/octarq-org/led/internal/models"
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
		if strings.HasPrefix(token, "led_") {
			hash := models.HashToken(token)
			var tok models.Token
			if h.db.Where("hash = ?", hash).First(&tok).Error == nil {
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
		return mcp_internal.NewServerInstance(h.db, orgID, h.plugins)
	}, &mcp.SSEOptions{
		DisableLocalhostProtection: true,
	})
	return h.mcpAuth(handler)
}

// mcpStreamHandler returns an http.Handler that handles MCP over Streamable HTTP.
func (h *Handler) mcpStreamHandler() http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		orgID := h.orgID(r)
		return mcp_internal.NewServerInstance(h.db, orgID, h.plugins)
	}, &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
	})
	return h.mcpAuth(handler)
}
