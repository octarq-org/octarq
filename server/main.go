// Command octarq is a single-binary domain / short-link / email management
// service (link · email · domain). It serves an embedded React dashboard,
// a JSON API, and a short-link redirector from one process.
//
// This is the open-core binary: it runs the app with no extra plugins. A
// downstream distribution reuses the same app package and
// registers additional plugins before Run — see the plugin package.
//
// Subcommands:
//
//	octarq          run the HTTP server (default)
//	octarq mcp      run the Model Context Protocol server over stdio, exposing
//	             workspace-scoped read-only short-link / email / domain tools to
//	             AI clients such as Claude Code, Claude Desktop and Cursor.
//	             See internal/mcp.
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
	"github.com/octarq-org/octarq/server/plugins/builtin"
)

func main() {
	// Structured JSON logging for the whole process. Edge access logs and the
	// app lifecycle logs both flow through this default logger. The severity
	// threshold is operator-configurable via OCTARQ_LOG_LEVEL.
	level, err := config.LogLevel()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	baseHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(&telemetry.TraceLogHandler{Handler: baseHandler}))

	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches every subcommand through the shared Cobra tree
// (server/cli) and returns the process exit code; main() only sets up
// logging and os.Exit's the code. ctx and writers are passed in so the
// dispatch logic and the default server boot stay unit-testable without
// forking a process.
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

// applyServerFlags maps `server` flag values onto process config before boot.
// Explicit flags win over the environment; a bad --config path fails closed.
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

// runMCPServer composes the Core plugins so their MCP tools (list_links,
// list_domains, list_mailboxes/emails, export_data) are registered on the
// stdio server.
func runMCPServer(ctx context.Context) int {
	if err := mcp.RunWithPlugins(ctx, builtin.Default()); err != nil {
		slog.Error("mcp failed", "err", err)
		return 1
	}
	return 0
}

// runOpenAPI prints the published specification. It boots the same
// composition the server does — Core plugins included — so the document is
// read off the live handler registrations rather than described alongside
// them. Passing nil here would emit a spec missing every links, mail, DNS
// and help route.
func runOpenAPI(stdout io.Writer) int {
	if err := openapi.Generate(stdout, builtin.Default()); err != nil {
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
	// Compose the OSS Core feature set. This is the composition root — Core
	// plugins are mounted the same way Pro plugins are (a.Use), and a trimmed
	// edition would build its own main that Uses a subset.
	for _, p := range builtin.Default() {
		a.Use(p)
	}
	// Compose the full-stack example plugin so the OSS demo binary ships a
	// complete, toggleable feature end-to-end: its Go half (hello.Plugin) pairs
	// with the @acme/octarq-plugin-hello UI half from the frontend manifest. It
	// implements plugin.Describer with EnabledByDefault, so the feature is on
	// for a fresh workspace yet stays user-toggleable (it is not Core) from
	// Settings → Plugins; the app's feature gate 404s its route and the host
	// drops its menu while the feature is turned off.
	a.Use(hello.Plugin{})
	// Compose any third-party plugins wired in at build time via the
	// OCTARQ_PLUGINS manifest (see custom_plugins.go + cmd/octarq-build). The
	// committed default is empty, so a plain build is unaffected.
	for _, p := range customPlugins() {
		a.Use(p)
	}
	startIPCDaemon(ctx, a)
	if err := a.Run(ctx); err != nil {
		slog.Error("run failed", "err", err)
		return 1
	}
	return 0
}

// startIPCDaemon serves the Connect-RPC control plane on the Unix socket in
// the background. It is best-effort: a socket failure is logged and the
// server keeps running, because observability must never take down serving.
func startIPCDaemon(ctx context.Context, a *app.App) {
	socketPath := ipc.SocketPath()
	lis, err := ipc.Listen(socketPath)
	if err != nil {
		slog.Error("ipc listen failed; status/top/reload unavailable", "socket", socketPath, "err", err)
		return
	}
	info := buildinfo.Get()
	daemon := newIPCDaemon(info.Version, a.HealthCollector)
	go func() {
		if err := daemon.Serve(ctx, lis); err != nil {
			slog.Error("ipc serve failed", "err", err)
		}
	}()
}

// newIPCDaemon wires daemon state into the control plane: version and start
// time are static, reload re-validates config and re-applies the log level,
// metrics reads the health collector (unknown when it has not started).
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
