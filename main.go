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
	"log"
	"os"

	"github.com/Jungley8/led/app"
	"github.com/Jungley8/led/internal/mcp"
)

func main() {
	// Dispatch subcommands before standing up the full server. `led mcp` runs a
	// stdio MCP server instead of the HTTP service.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := mcp.Run(context.Background()); err != nil {
			log.Fatalf("mcp: %v", err)
		}
		return
	}

	a, err := app.New()
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	if err := a.Run(context.Background()); err != nil {
		log.Fatalf("run: %v", err)
	}
}
