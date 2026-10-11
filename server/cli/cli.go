// Package cli owns the octarq Cobra command tree: every subcommand is a
// cobra.Command, and main.go only wires dependencies and exits with the
// returned code. Leaf bodies delegate to the long-standing run* functions so
// exit-code contracts stay pinned by the existing CLI tests.
//
// Tree:
//
//	octarq                  boot the HTTP/Web service (default)
//	octarq server           boot the service with --port/--host/--config flags
//	octarq mcp              stdio MCP server
//	octarq openapi          print the OpenAPI document
//	octarq backup/restore   database disaster-recovery pair
//	octarq plugin new       plugin scaffolder (legacy ordering preserved)
//	octarq service          OS service lifecycle via kardianos/service
//	octarq completion       shell completion scripts
//	octarq version          build metadata
//
// Downstream binaries (octarq-pro) reuse NewRootCmd and mount their own
// commercial subcommands through Deps.Extra.
//
// This file retains root command tree construction (NewRootCmd), dependency
// injection wiring (Deps), and top-level process execution (Execute).
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Deps wires a binary's behavior into the shared command tree. Every func
// returns the process exit code for its command.
type Deps struct {
	// Name is the binary name shown in usage ("octarq" or "octarq-pro").
	Name string
	// PrintVersion writes build metadata to w (e.g. "octarq v1.2.3 (...)").
	PrintVersion func(w io.Writer)
	// Boot starts the HTTP/Web service and blocks until shutdown.
	Boot func(ctx context.Context) int
	// MCP runs the stdio MCP server instead of the HTTP service.
	MCP func(ctx context.Context) int
	// OpenAPI writes the published specification to w.
	OpenAPI func(w io.Writer) int
	// Backup / Restore run the disaster-recovery pair with raw args.
	Backup  func(args []string) int
	Restore func(args []string) int
	// Plugin handles the whole `plugin` subtree (legacy ordering kept).
	Plugin func(args []string) int
	// ApplyServerFlags maps `server` flag values onto process config
	// (OCTARQ_LISTEN / dotenv). Nil keeps flags accepted but inert.
	ApplyServerFlags func(port, host, configPath string) error
	// Service describes the OS service unit. Empty Name disables `service`.
	ServiceName    string
	ServiceDisplay string
	ServiceDesc    string
	// Extra mounts binary-specific subcommands (e.g. Pro seed-catalog).
	Extra func(root *cobra.Command)
}

// exitError carries a command body's exit code through cobra's error return.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func codeErr(code int) error {
	if code == 0 {
		return nil
	}
	return &exitError{code: code}
}

// NewRootCmd builds the shared command tree. stdout/stderr writers are bound
// at Execute time so tests can capture output without forking a process.
func NewRootCmd(d Deps) *cobra.Command {
	name := d.Name
	if name == "" {
		name = "octarq"
	}
	root := &cobra.Command{
		Use:   name,
		Short: "Single-binary ops platform (links · email · domains)",
		Long: `Octarq is a single-binary ops platform serving an embedded dashboard,` +
			` a JSON API, and a short-link redirector from one process.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Boot == nil {
				return fmt.Errorf("server is not supported by %s", name)
			}
			return codeErr(d.Boot(cmd.Context()))
		},
	}
	root.AddCommand(
		newServerCmd(d),
		newMCPCommand(d),
		newOpenAPICommand(d),
		newBackupCommand(d),
		newRestoreCommand(d),
		newPluginCommand(d),
		newStatusCommand(),
		newReloadCommand(),
		newTopCommand(),
		newSetupCommand(),
		newVersionCommand(d),
		newCompletionCommand(name),
	)
	if svc := newServiceCommand(d); svc != nil {
		root.AddCommand(svc)
	}
	if d.Extra != nil {
		d.Extra(root)
	}
	return root
}

// Execute runs the tree and maps the outcome to a process exit code.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, d Deps) int {
	// Keep the historical `--version` / `-version` prefix form working: it
	// prints to stdout, leaves stderr empty, and exits 0.
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		if d.PrintVersion != nil {
			d.PrintVersion(stdout)
		}
		return 0
	}
	root := NewRootCmd(d)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		if ee, ok := err.(*exitError); ok {
			return ee.code
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
