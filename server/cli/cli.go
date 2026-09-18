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
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kardianos/service"
	"github.com/octarq-org/octarq/server/clitop"
	"github.com/octarq-org/octarq/server/internal/ipc"
	"github.com/spf13/cobra"
)

func formatDuration(secs int64) string {
	d := time.Duration(secs) * time.Second
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func formatBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

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

func newServerCmd(d Deps) *cobra.Command {
	var port, host, configPath string
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the HTTP/Web service (default command)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Boot == nil {
				return fmt.Errorf("server is not supported by %s", cmd.Root().Name())
			}
			if d.ApplyServerFlags != nil {
				if err := d.ApplyServerFlags(port, host, configPath); err != nil {
					return err
				}
			}
			return codeErr(d.Boot(cmd.Context()))
		},
	}
	cmd.Flags().StringVar(&port, "port", "", "TCP port to listen on (overrides OCTARQ_LISTEN)")
	cmd.Flags().StringVar(&host, "host", "", "interface to bind (overrides OCTARQ_LISTEN)")
	cmd.Flags().StringVar(&configPath, "config", "", "dotenv file to load before boot (Fail-Closed on error)")
	return cmd
}

func newMCPCommand(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run the Model Context Protocol server over stdio",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.MCP == nil {
				return fmt.Errorf("mcp is not supported by %s", cmd.Root().Name())
			}
			return codeErr(d.MCP(cmd.Context()))
		},
	}
}

func newOpenAPICommand(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "openapi",
		Short: "Print the published OpenAPI specification",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.OpenAPI == nil {
				return fmt.Errorf("openapi is not supported by %s", cmd.Root().Name())
			}
			return codeErr(d.OpenAPI(cmd.OutOrStdout()))
		},
	}
}

func newBackupCommand(d Deps) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up the database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Backup == nil {
				return fmt.Errorf("backup is not supported by %s", cmd.Root().Name())
			}
			args := []string{}
			if out != "" {
				args = []string{"--out", out}
			}
			return codeErr(d.Backup(args))
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output file path for backup")
	return cmd
}

func newRestoreCommand(d Deps) *cobra.Command {
	var in string
	var yes bool
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore the database from a backup file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Restore == nil {
				return fmt.Errorf("restore is not supported by %s", cmd.Root().Name())
			}
			var args []string
			if in != "" {
				args = append(args, "--in", in)
			}
			if yes {
				args = append(args, "--yes")
			}
			return codeErr(d.Restore(args))
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "input backup file path (required)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm restore without prompt")
	_ = cmd.Flags().Bool("confirm", false, "alias of --yes")
	return cmd
}

func newPluginCommand(d Deps) *cobra.Command {
	// The `plugin new` argument orderings (name-first and flags-first) are
	// legacy behavior pinned by CLI tests; keep stdlib flag parsing inside
	// the existing body and route the raw remainder through untouched.
	cmd := &cobra.Command{
		Use:                "plugin",
		Short:              "Plugin utilities (new: scaffold a plugin skeleton)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.Plugin == nil {
				return fmt.Errorf("plugin is not supported by %s", cmd.Root().Name())
			}
			if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
				fmt.Fprintf(cmd.OutOrStdout(), "usage: %s plugin new <name> [flags]\n", cmd.Root().Name())
				return nil
			}
			return codeErr(d.Plugin(args))
		},
	}
	return cmd
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon vitals via the control socket",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			reply, err := ipc.NewClient(ipc.SocketPath()).Status(ctx)
			if err != nil {
				return fmt.Errorf("daemon unreachable: %w", err)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "version: %s  uptime: %s  pid: %d\n", reply.Version, formatDuration(reply.UptimeSecs), reply.PID)
			fmt.Fprintf(out, "goroutines: %d  mem: %s\n", reply.Goroutines, formatBytes(reply.MemAlloc))
			return nil
		},
	}
}

func newReloadCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Ask the daemon to re-read safe config keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			reply, err := ipc.NewClient(ipc.SocketPath()).Reload(ctx)
			if err != nil {
				return fmt.Errorf("daemon unreachable: %w", err)
			}
			if len(reply.Applied) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "reloaded (no keys changed)")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reloaded: %s\n", strings.Join(reply.Applied, ", "))
			return nil
		},
	}
}

func newTopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "top",
		Short: "Live terminal dashboard for daemon vitals (plain text when piped)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return codeErr(clitop.RunTop(cmd.Context(), cmd.OutOrStdout(), ipc.SocketPath()))
		},
	}
}

func newSetupCommand() *cobra.Command {
	var envPath string
	var force bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive first-boot wizard (writes .env)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return codeErr(clitop.RunSetup(cmd.Context(), cmd.OutOrStdout(), envPath, force))
		},
	}
	cmd.Flags().StringVar(&envPath, "env-path", ".env", "dotenv file to write")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing non-empty file")
	return cmd
}

func newVersionCommand(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build metadata",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.PrintVersion != nil {
				d.PrintVersion(cmd.OutOrStdout())
			}
			return nil
		},
	}
}

func newCompletionCommand(name string) *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(out)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletion(out)
			default:
				return fmt.Errorf("unsupported shell %q (try: bash, zsh, fish, powershell)", args[0])
			}
		},
	}
}

// program adapts a blocking Boot func to the service.Interface lifecycle.
type program struct {
	boot   func(ctx context.Context) int
	cancel context.CancelFunc
	done   chan int
}

func (p *program) Start(_ service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan int, 1)
	go func() { p.done <- p.boot(ctx) }()
	return nil
}

func (p *program) Stop(_ service.Service) error {
	if p.cancel != nil {
		p.cancel()
		<-p.done
	}
	return nil
}

func newServiceCommand(d Deps) *cobra.Command {
	if d.ServiceName == "" || d.Boot == nil {
		return nil
	}
	prg := &program{boot: d.Boot}
	svcConfig := &service.Config{
		Name:        d.ServiceName,
		DisplayName: d.ServiceDisplay,
		Description: d.ServiceDesc,
	}
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the OS system service (install, start, stop, status, …)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceBackend(prg, svcConfig)
			if err != nil {
				return fmt.Errorf("service not supported on this platform: %w", err)
			}
			return runServiceAction(cmd.OutOrStdout(), svc, args[0])
		},
	}
	return cmd
}

// serviceBackend abstracts kardianos/service for testability.
type serviceBackend interface {
	Install() error
	Uninstall() error
	Start() error
	Stop() error
	Restart() error
	Status() (service.Status, error)
}

// newServiceBackend constructs the platform backend. It is a variable so
// tests can inject a fake without touching the OS service manager.
var newServiceBackend = func(prg *program, cfg *service.Config) (serviceBackend, error) {
	return service.New(prg, cfg)
}

func runServiceAction(out io.Writer, svc serviceBackend, action string) error {
	switch action {
	case "install":
		if err := svc.Install(); err != nil {
			return err
		}
		fmt.Fprintln(out, "service installed")
	case "uninstall":
		if err := svc.Uninstall(); err != nil {
			return err
		}
		fmt.Fprintln(out, "service uninstalled")
	case "start":
		if err := svc.Start(); err != nil {
			return err
		}
		fmt.Fprintln(out, "service started")
	case "stop":
		if err := svc.Stop(); err != nil {
			return err
		}
		fmt.Fprintln(out, "service stopped")
	case "restart":
		if err := svc.Restart(); err != nil {
			return err
		}
		fmt.Fprintln(out, "service restarted")
	case "status":
		status, err := svc.Status()
		if err != nil {
			return err
		}
		switch status {
		case service.StatusRunning:
			fmt.Fprintln(out, "running")
		case service.StatusStopped:
			fmt.Fprintln(out, "stopped")
		default:
			fmt.Fprintln(out, "unknown")
		}
	default:
		return fmt.Errorf("unknown service action %q (try: install, uninstall, start, stop, restart, status)", action)
	}
	return nil
}
