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
//	             read-only short-link / email / domain tools (plus a guarded
//	             read-only SQL tool) to AI clients such as Claude Code, Claude
//	             Desktop and Cursor. See internal/mcp.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/octarq-org/octarq/app"
	hello "github.com/octarq-org/octarq/examples/plugin-hello"
	"github.com/octarq-org/octarq/internal/mcp"
	"github.com/octarq-org/octarq/openapi"
	"github.com/octarq-org/octarq/plugins/builtin"
)

func main() {
	// Structured JSON logging for the whole process. Edge access logs and the
	// app lifecycle logs both flow through this default logger.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	// Dispatch subcommands before standing up the full server. `octarq mcp` runs a
	// stdio MCP server instead of the HTTP service.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		// Compose the Core plugins so their MCP tools (list_links, list_domains,
		// list_mailboxes/emails, export_data) are registered on the stdio server.
		if err := mcp.RunWithPlugins(context.Background(), builtin.Default()); err != nil {
			slog.Error("mcp failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "openapi" {
		if err := openapi.Generate(os.Stdout, nil); err != nil {
			slog.Error("openapi generation failed", "err", err)
			os.Exit(1)
		}
		return
	}

	// `octarq plugin new <name>` scaffolds a plugin skeleton (Go + web halves)
	// and exits, without standing up the server.
	if len(os.Args) > 1 && os.Args[1] == "plugin" {
		os.Exit(runPluginCommand(os.Args[2:]))
	}

	a, err := app.New()
	if err != nil {
		slog.Error("init failed", "err", err)
		os.Exit(1)
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
	// has no Describer, so it is a user-toggleable feature — off by default,
	// opt-in from Settings → Plugins, and its /hello menu is backend-gated.
	a.Use(hello.Plugin{})
	// Compose any third-party plugins wired in at build time via the
	// OCTARQ_PLUGINS manifest (see custom_plugins.go + cmd/octarq-build). The
	// committed default is empty, so a plain build is unaffected.
	for _, p := range customPlugins() {
		a.Use(p)
	}
	if err := a.Run(context.Background()); err != nil {
		slog.Error("run failed", "err", err)
		os.Exit(1)
	}
}
