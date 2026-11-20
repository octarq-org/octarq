package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/cache"
	"github.com/octarq-org/octarq/server/internal/db"
	"github.com/octarq-org/octarq/server/internal/endpoint"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

var errNoOrgInContext = errors.New("no workspace in this request")

const version = "0.1.0"

const stdioOrgID uint = 1

type server struct {
	gdb    *gorm.DB
	orgID  uint
	lookup func(name string) (any, bool)
	srv    *mcp.Server
	subs   *subscriptionTracker
}

func Run(ctx context.Context) error {
	return RunWithPlugins(ctx, nil, nil)
}

func NewNetworkedServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, lookup func(string) (any, bool)) *mcp.Server {
	return buildServerInstance(gdb, orgID, plugins, lookup, false, nil, nil)
}

func NewServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, gate plugin.TierGate) *mcp.Server {
	return buildServerInstance(gdb, orgID, plugins, nil, true, gate, nil)
}

type mcpCryptoHost struct {
	plugin.Host
	crypto plugin.CryptoVault
}

func (h mcpCryptoHost) Crypto() plugin.CryptoVault { return h.crypto }

func NewStdioPluginHost(pctx *plugin.Context, baseHost plugin.Host) plugin.Host {
	ensured := plugin.EnsureHost(pctx)
	if baseHost != nil && baseHost.Crypto() != nil {
		return mcpCryptoHost{Host: ensured, crypto: baseHost.Crypto()}
	}
	return ensured
}

func buildServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, lookup func(string) (any, bool), mountPlugins bool, gate plugin.TierGate, baseHost plugin.Host) *mcp.Server {
	lookupFn := lookup
	endpointEngine := endpoint.NewEngine()
	if mountPlugins {
		reg := plugin.NewRegistry()
		cacheBackend := cache.New("")
		pctx := &plugin.Context{
			DB:               gdb,
			Provide:          reg.Provide,
			Lookup:           reg.Lookup,
			Cache:            cache.NewScoped(cacheBackend, "mcp"),
			RegisterEndpoint: endpointEngine.Register,
			RegisterTenantView: func(view plugin.TenantView) {
				if err := tenantsql.DefaultRegistry().Register(view); err != nil {
					slog.Warn("mcp: failed to register tenant view", "name", view.Name, "err", err)
				}
			},
			RegisterReactor: eventbus.RegisterReactor,
		}
		pctx.Host = NewStdioPluginHost(pctx, baseHost)
		for _, p := range plugins {
			pctxCopy := *pctx
			pctxCopy.Cache = cache.NewScoped(cacheBackend, p.Name())
			p.Mount(nil, &pctxCopy)
		}
		if lookupFn == nil {
			lookupFn = reg.Lookup
		}
	}

	s := &server{gdb: gdb, orgID: orgID, lookup: lookupFn, subs: &subscriptionTracker{}}

	impl := &mcp.Implementation{Name: "octarq", Version: version}
	opts := &mcp.ServerOptions{
		Instructions: "octarq is a self-hosted one-person-company backend. These tools " +
			"read/write short links, email, and domains. Everything is scoped to the " +
			"caller's own workspace.",
		SubscribeHandler:   s.handleSubscribe,
		UnsubscribeHandler: s.handleUnsubscribe,
	}
	srv := mcp.NewServer(impl, opts)
	s.srv = srv
	s.registerTools(srv)
	s.registerResources(srv)
	s.registerExtraResources(srv)
	if err := endpointEngine.MountMCP(srv, gate); err != nil {
		slog.Warn("mcp: failed to mount endpoint engine tools", "err", err)
	}

	for _, p := range plugins {
		if mp, ok := p.(plugin.MCPProvider); ok && gate.Allows(mp.MCPTier()) {
			mp.RegisterMCP(srv)
		}
	}
	return srv
}

func RunWithPlugins(ctx context.Context, plugins []plugin.Plugin, gate plugin.TierGate) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}
	gdb, err := db.Open(cfg)
	if err != nil {
		return fmt.Errorf("mcp: open db: %w", err)
	}

	telemetry.IncMCPSessions()
	defer telemetry.DecMCPSessions()

	ctx = plugin.WithOrgID(ctx, stdioOrgID)
	ctx = plugin.WithTokenRole(ctx, "owner")
	srv := NewServerInstance(gdb, stdioOrgID, plugins, gate)
	return srv.Run(ctx, stdioTransport())
}

func RunStdio(ctx context.Context, gdb *gorm.DB, plugins []plugin.Plugin, host plugin.Host, gate plugin.TierGate) error {
	telemetry.IncMCPSessions()
	defer telemetry.DecMCPSessions()

	ctx = plugin.WithOrgID(ctx, stdioOrgID)
	ctx = plugin.WithTokenRole(ctx, "owner")
	srv := buildServerInstance(gdb, stdioOrgID, plugins, nil, true, gate, host)
	return srv.Run(ctx, stdioTransport())
}

func stdioTransport() mcp.Transport {
	return &mcp.IOTransport{Reader: openReader{os.Stdin}, Writer: openWriter{os.Stdout}}
}

type openReader struct{ io.Reader }

func (openReader) Close() error { return nil }

type openWriter struct{ io.Writer }

func (openWriter) Close() error { return nil }

func (s *server) registerTools(srv *mcp.Server) {
	plugin.AddMCPTool(srv, "core_mcp", &mcp.Tool{
		Name:        "octarq_infra__export_data",
		Description: "Export the operator's data for one resource type (links, emails, domains, mailboxes) as JSON or 'all'/'zip_url' for a full ZIP summary with signed download URL.",
	}, s.exportData)
}

func jsonResult[T any](v T) (*mcp.CallToolResult, any, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
	}, v, nil
}
