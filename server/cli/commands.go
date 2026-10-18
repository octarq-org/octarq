package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/clitop"
	"github.com/octarq-org/octarq/server/internal/ipc"
	"github.com/spf13/cobra"
)

// Subcommand constructors for standalone CLI actions.
//
// This file groups the constructors for child commands attached to the root CLI:
// server, mcp, openapi, backup, restore, plugin, status, reload, top, setup,
// version, and shell completion.

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
	var remote bool
	var encrypt bool
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up the database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if d.Backup == nil {
				return fmt.Errorf("backup is not supported by %s", cmd.Root().Name())
			}
			args := []string{}
			if out != "" {
				args = append(args, "--out", out)
			}
			if remote {
				args = append(args, "--remote")
			}
			if cmd.Flags().Changed("encrypt") {
				if encrypt {
					args = append(args, "--encrypt=true")
				} else {
					args = append(args, "--encrypt=false")
				}
			}
			return codeErr(d.Backup(args))
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output file path for backup")
	cmd.Flags().BoolVar(&remote, "remote", false, "upload encrypted backup to configured remote S3/R2 storage")
	cmd.Flags().BoolVar(&encrypt, "encrypt", true, "encrypt backup with AES-256-GCM before saving/uploading")
	return cmd
}

func newRestoreCommand(d Deps) *cobra.Command {
	var in string
	var fromS3 string
	var decrypt bool
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
			if fromS3 != "" {
				args = append(args, "--from-s3", fromS3)
			}
			if decrypt {
				args = append(args, "--decrypt")
			}
			if yes {
				args = append(args, "--yes")
			}
			return codeErr(d.Restore(args))
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "input backup file path (required unless --from-s3 is used)")
	cmd.Flags().StringVar(&fromS3, "from-s3", "", "restore directly from remote S3 snapshot key (or 'latest')")
	cmd.Flags().BoolVar(&decrypt, "decrypt", false, "decrypt backup file using OCTARQ_SECRET_KEY before restoring")
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
