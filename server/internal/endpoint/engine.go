package endpoint

import (
	"context"
	"fmt"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

type Engine struct {
	mu        sync.RWMutex
	endpoints []plugin.Endpoint
}

func NewEngine() *Engine {
	return &Engine{}
}

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

func (e *Engine) Endpoints() []plugin.Endpoint {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]plugin.Endpoint, len(e.endpoints))
	copy(out, e.endpoints)
	return out
}

func (e *Engine) Lookup(name string) (plugin.Endpoint, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, ep := range e.endpoints {
		if ep.EndpointName() == name {
			return ep, true
		}
	}
	return nil, false
}

type HTTPOptions struct {
	RequireAuth func(ctx context.Context) (uint, error)
	RequireRole func(ctx context.Context, roles []string) error
}

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

func (e *Engine) MountMCP(srv *mcp.Server, gate plugin.TierGate) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, ep := range e.endpoints {
		if ep.EndpointExposeMCP() && gate.Allows(ep.EndpointTier()) {
			if mr, ok := ep.(plugin.MCPRegistrar); ok {
				if err := mr.RegisterMCP(srv); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
