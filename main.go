// Command octarq is a single-binary domain / short-link / email management
// service (link · email · domain). It serves an embedded React dashboard,
// a JSON API, and a short-link redirector from one process.
//
// This is the open-core binary: it runs the app with no Pro plugins. The
// commercial build (private octarq-core module) reuses the same app package and
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

	"github.com/octarq-org/octarq/app"
	"github.com/octarq-org/octarq/config"
	hello "github.com/octarq-org/octarq/examples/plugin-hello"
	"github.com/octarq-org/octarq/internal/buildinfo"
	"github.com/octarq-org/octarq/internal/mcp"
	"github.com/octarq-org/octarq/openapi"
	"github.com/octarq-org/octarq/plugins/builtin"
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches every subcommand and returns the process exit code; main()
// only sets up logging and os.Exit's the code. The body was extracted from
// main() so the dispatch logic and the default server boot can be exercised by
// unit tests without forking a process — ctx and writers are passed in.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	// Print build metadata and exit. Version/commit are injected at build time
	// (see Makefile's LDFLAGS); outside a git checkout they degrade to dev /
	// unknown, which is also what `go run .` reports.
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		info := buildinfo.Get()
		fmt.Fprintf(stdout, "octarq %s (commit %s, built %s)\n", info.Version, info.Commit, info.BuiltAt)
		return 0
	}

	// Dispatch subcommands before standing up the full server. `octarq mcp` runs a
	// stdio MCP server instead of the HTTP service.
	if len(args) > 0 && args[0] == "mcp" {
		// Compose the Core plugins so their MCP tools (list_links, list_domains,
		// list_mailboxes/emails, export_data) are registered on the stdio server.
		if err := mcp.RunWithPlugins(context.Background(), builtin.Default()); err != nil {
			slog.Error("mcp failed", "err", err)
			return 1
		}
		return 0
	}

	if len(args) > 0 && args[0] == "openapi" {
		if err := openapi.Generate(stdout, nil); err != nil {
			slog.Error("openapi generation failed", "err", err)
			return 1
		}
		return 0
	}

	if len(args) > 0 && args[0] == "backup" {
		return runBackupCommand(args[1:])
	}

	if len(args) > 0 && args[0] == "restore" {
		return runRestoreCommand(args[1:])
	}

	// `octarq plugin new <name>` scaffolds a plugin skeleton (Go + web halves)
	// and exits, without standing up the server.
	if len(args) > 0 && args[0] == "plugin" {
		return runPluginCommand(args[1:])
	}

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
	if err := a.Run(ctx); err != nil {
		slog.Error("run failed", "err", err)
		return 1
	}
	return 0
}
