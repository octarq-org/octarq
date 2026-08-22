package endpoint

import (
	"context"
	"fmt"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
)

// Engine collects declarative endpoints and mounts them onto HTTP (Huma) and MCP servers.
type Engine struct {
	mu        sync.RWMutex
	endpoints []plugin.Endpoint
}

// NewEngine creates a new declarative endpoint engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Register adds an endpoint specification to the engine.
func (e *Engine) Register(spec any) error {
	ep, ok := spec.(plugin.Endpoint)
	if !ok {
		return fmt.Errorf("endpoint spec must implement plugin.Endpoint, got %T", spec)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.endpoints = append(e.endpoints, ep)
	return nil
}

// Endpoints returns all registered endpoints.
func (e *Engine) Endpoints() []plugin.Endpoint {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]plugin.Endpoint, len(e.endpoints))
	copy(out, e.endpoints)
	return out
}

// HTTPOptions provides authentication and role checking hooks for HTTP endpoint execution.
type HTTPOptions struct {
	RequireAuth func(ctx context.Context) (uint, error)
	RequireRole func(ctx context.Context, roles []string) error
}

// MountHTTP registers all collected endpoints onto the given Huma API.
func (e *Engine) MountHTTP(api huma.API, opts HTTPOptions) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, ep := range e.endpoints {
		if hr, ok := ep.(plugin.HTTPRegistrar); ok {
			if err := hr.RegisterHTTP(api, plugin.HTTPOptions{
				RequireAuth: opts.RequireAuth,
				RequireRole: opts.RequireRole,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// MountMCP registers all collected endpoints with ExposeMCP=true onto the MCP server.
func (e *Engine) MountMCP(srv *mcp.Server) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, ep := range e.endpoints {
		if ep.EndpointExposeMCP() {
			if mr, ok := ep.(plugin.MCPRegistrar); ok {
				if err := mr.RegisterMCP(srv); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
