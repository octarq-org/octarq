package openapi

// extraPaths describes the core routes registered directly on the mux rather
// than through huma, which is the only reason they need describing by hand.
//
// Keep this list minimal and keep it honest: everything here is a maintenance
// liability of exactly the kind generating the rest of the document removed. An
// entry earns its place only while its route cannot be a huma operation —
// OAuth's redirect handshake and the MCP transports are streaming or
// browser-redirect endpoints, not JSON request/response pairs.
//
// The merge in Marshal only fills gaps, so if one of these ever becomes a huma
// operation the generated version wins automatically and the stale entry here
// is inert rather than wrong.
func extraPaths() map[string]any {
	errorResponse := map[string]any{
		"description": "Error",
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/Error"},
			},
		},
	}
	return map[string]any{
		"/auth/begin/{provider}": map[string]any{
			"get": map[string]any{
				"tags":        []any{"Auth"},
				"summary":     "Begin an OAuth sign-in",
				"description": "Redirects the browser to the named identity provider. Answers 503 when no provider is configured, or when the request arrived on a hostname this instance cannot build a callback URL for. Browser-only: there is no JSON response to consume.",
				"security":    []any{},
				"parameters": []any{
					map[string]any{
						"name":     "provider",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"302":     map[string]any{"description": "Redirect to the provider"},
					"503":     map[string]any{"description": "No provider configured for this hostname"},
					"default": errorResponse,
				},
			},
		},
		"/auth/callback/{provider}": map[string]any{
			"get": map[string]any{
				"tags":        []any{"Auth"},
				"summary":     "Complete an OAuth sign-in",
				"description": "The provider's redirect target. On success it establishes the session cookie and redirects into the dashboard. Browser-only.",
				"security":    []any{},
				"parameters": []any{
					map[string]any{
						"name":     "provider",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"302":     map[string]any{"description": "Redirect into the dashboard"},
					"default": errorResponse,
				},
			},
		},
		"/api/mcp/sse": map[string]any{
			"get": map[string]any{
				"tags":        []any{"MCP"},
				"summary":     "Model Context Protocol (SSE transport)",
				"description": "Server-Sent Events transport for the MCP server, exposing read-only short-link, email and domain tools to AI clients. Authenticate with a bearer API token. Not a JSON endpoint: the response is an open event stream.",
				"responses": map[string]any{
					"200":     map[string]any{"description": "SSE stream"},
					"default": errorResponse,
				},
			},
		},
		"/api/mcp/stream": map[string]any{
			"post": map[string]any{
				"tags":        []any{"MCP"},
				"summary":     "Model Context Protocol (streamable HTTP transport)",
				"description": "Streamable-HTTP transport for the MCP server. Authenticate with a bearer API token. The body is JSON-RPC framed by MCP, not an octarq resource.",
				"responses": map[string]any{
					"200":     map[string]any{"description": "MCP stream"},
					"default": errorResponse,
				},
			},
		},
	}
}
