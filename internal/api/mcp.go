package api

import (
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/internal/auth"
	mcp_internal "github.com/octarq-org/octarq/internal/mcp"
	"github.com/octarq-org/octarq/internal/models"
)

// mcpAuth is a middleware that authenticates MCP requests. An authenticated
// session passes; otherwise a valid ?token= query parameter is accepted.
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
			// tok.OrgID != 0 is a deliberate fail-closed guard: the networked MCP
			// tools default an unresolved org to 1 (the stdio-CLI single-tenant
			// default), so a token that somehow carries owner_id 0 must never be
			// admitted here — it would read tenant 1's data. Legitimate tokens can't
			// have owner_id 0 (the column defaults to 1), so this only rejects
			// corrupt rows.
			if h.db.Where("hash = ?", hash).First(&tok).Error == nil && !tok.Expired() && tok.OrgID != 0 {
				// Stamp the token identity, not just the org: without it an
				// MCP-driven mutation lands in the audit log with actor 0 and no
				// tokenId, i.e. attributable to nobody at all.
				ctx := auth.WithOrgID(r.Context(), tok.OrgID)
				ctx = h.auth.WithTokenIdentity(ctx, tok)
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
		return mcp_internal.NewNetworkedServerInstance(h.db, orgID, h.plugins, h.LookupService)
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
		return mcp_internal.NewNetworkedServerInstance(h.db, orgID, h.plugins, h.LookupService)
	}, &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
	})
	return h.mcpAuth(handler)
}
