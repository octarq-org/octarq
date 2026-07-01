// Command led is a single-binary domain / short-link / email management
// service (link · email · domain). It serves an embedded React dashboard,
// a JSON API, and a short-link redirector from one process.
//
// This is the open-core binary: it runs the app with no Pro plugins. The
// commercial build (private led-core module) reuses the same app package and
// registers additional plugins before Run — see the plugin package.
//
// Subcommands:
//
//	led          run the HTTP server (default)
//	led mcp      run the Model Context Protocol server over stdio, exposing
//	             read-only short-link / email / domain tools (plus a guarded
//	             read-only SQL tool) to AI clients such as Claude Code, Claude
//	             Desktop and Cursor. See internal/mcp.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/Jungley8/led/app"
	"github.com/Jungley8/led/internal/mcp"
)

func main() {
	// Structured JSON logging for the whole process. Edge access logs and the
	// app lifecycle logs both flow through this default logger.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	// Dispatch subcommands before standing up the full server. `led mcp` runs a
	// stdio MCP server instead of the HTTP service.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := mcp.Run(context.Background()); err != nil {
			slog.Error("mcp failed", "err", err)
			os.Exit(1)
		}
		return
	}

	a, err := app.New()
	if err != nil {
		slog.Error("init failed", "err", err)
		os.Exit(1)
	}
	if err := a.Run(context.Background()); err != nil {
		slog.Error("run failed", "err", err)
		os.Exit(1)
	}
}
