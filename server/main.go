package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/app"
	"github.com/octarq-org/octarq/server/cli"
	"github.com/octarq-org/octarq/server/config"
	hello "github.com/octarq-org/octarq/server/examples/plugin-hello"
	"github.com/octarq-org/octarq/server/internal/buildinfo"
	"github.com/octarq-org/octarq/server/internal/ipc"
	"github.com/octarq-org/octarq/server/internal/mcp"
	"github.com/octarq-org/octarq/server/internal/monitor"
	"github.com/octarq-org/octarq/server/openapi"
	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/builtin"
)

func main() {
	level, err := config.LogLevel()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	baseHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(&telemetry.TraceLogHandler{Handler: baseHandler}))

	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return cli.Execute(ctx, args, stdout, stderr, cli.Deps{
		Name: "octarq",
		PrintVersion: func(w io.Writer) {
			info := buildinfo.Get()
			fmt.Fprintf(w, "octarq %s (commit %s, built %s)\n", info.Version, info.Commit, info.BuiltAt)
		},
		Boot:    bootServer,
		MCP:     runMCPServer,
		OpenAPI: runOpenAPI,
		Backup:  runBackupCommand,
		Restore: runRestoreCommand,
		Plugin:  runPluginCommand,
		ApplyServerFlags: func(port, host, configPath string) error {
			return applyServerFlags(port, host, configPath)
		},
		ServiceName:    "octarq",
		ServiceDisplay: "Octarq Ops Platform",
		ServiceDesc:    "Octarq single-binary ops platform (links, email, domains).",
	})
}

func applyServerFlags(port, host, configPath string) error {
	if configPath != "" {
		if err := config.LoadDotEnv(configPath); err != nil {
			return fmt.Errorf("load --config %q: %w", configPath, err)
		}
	}
	if port == "" && host == "" {
		return nil
	}
	listen := os.Getenv("OCTARQ_LISTEN")
	if listen == "" {
		listen = ":8080"
	}
	baseHost, basePort := splitListen(listen)
	if host != "" {
		baseHost = host
	}
	if port != "" {
		basePort = strings.TrimPrefix(port, ":")
	}
	if baseHost == "" {
		baseHost = ":"
	}
	if strings.HasSuffix(baseHost, ":") {
		_ = os.Setenv("OCTARQ_LISTEN", baseHost+basePort)
	} else {
		_ = os.Setenv("OCTARQ_LISTEN", baseHost+":"+basePort)
	}
	return nil
}

func splitListen(listen string) (host, port string) {
	if i := strings.LastIndex(listen, ":"); i >= 0 {
		return listen[:i], listen[i+1:]
	}
	return listen, ""
}

func composePluginsForCLI() []plugin.Plugin {
	plugins := builtin.Default()
	return append(plugins, customPlugins()...)
}

func runMCPServer(ctx context.Context) int {
	return runMCPServerWith(ctx, composePluginsForCLI())
}

func runMCPServerWith(ctx context.Context, plugins []plugin.Plugin) int {
	if _, err := config.Load(); err != nil {
		slog.Error("mcp failed", "err", err)
		return 1
	}
	if err := plugin.ValidateAll(ctx, plugins, plugin.EnsureHost(nil)); err != nil {
		slog.Error("mcp failed", "err", err)
		return 1
	}
	if err := mcp.RunWithPlugins(ctx, plugins, nil); err != nil {
		slog.Error("mcp failed", "err", err)
		return 1
	}
	return 0
}

func runOpenAPI(stdout io.Writer) int {
	if err := openapi.Generate(stdout, composePluginsForCLI()); err != nil {
		slog.Error("openapi generation failed", "err", err)
		return 1
	}
	return 0
}

func bootServer(ctx context.Context) int {
	a, err := app.New()
	if err != nil {
		slog.Error("init failed", "err", err)
		return 1
	}
	for _, p := range builtin.Default() {
		a.Use(p)
	}
	a.Use(hello.Plugin{})
	for _, p := range customPlugins() {
		a.Use(p)
	}
	startIPCDaemon(ctx, buildinfo.Get().Version, a.HealthCollector)
	if err := a.Run(ctx); err != nil {
		slog.Error("run failed", "err", err)
		return 1
	}
	return 0
}

func startIPCDaemon(ctx context.Context, version string, collector func() *monitor.Collector) {
	socketPath := ipc.SocketPath()
	lis, err := ipc.Listen(socketPath)
	if err != nil {
		slog.Error("ipc listen failed; status/top/reload unavailable", "socket", socketPath, "err", err)
		return
	}
	daemon := newIPCDaemon(version, collector)
	go func() {
		if err := daemon.Serve(ctx, lis); err != nil {
			slog.Error("ipc serve failed", "err", err)
		}
	}()
}

func newIPCDaemon(version string, collector func() *monitor.Collector) *ipc.Daemon {
	started := time.Now()
	return &ipc.Daemon{
		Version: version,
		Started: started,
		OnReload: func(ctx context.Context) ([]string, error) {
			if _, err := config.Load(); err != nil {
				return nil, err
			}
			level, err := config.LogLevel()
			if err != nil {
				return nil, err
			}
			baseHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
			slog.SetDefault(slog.New(baseHandler))
			return []string{"log.level"}, nil
		},
		OnMetrics: func(ctx context.Context) (*ipc.MetricsReply, error) {
			c := collector()
			if c == nil {
				return &ipc.MetricsReply{Overall: "unknown"}, nil
			}
			report := c.LatestReport(ctx)
			rep := &ipc.MetricsReply{
				Overall:   string(report.Overall),
				CheckedAt: report.CheckedAt,
			}
			for _, p := range report.Providers {
				rep.Providers = append(rep.Providers, ipc.ProviderMetric{
					Name:   p.Name,
					Status: string(p.Status),
					Detail: p.Message,
				})
			}
			return rep, nil
		},
	}
}
