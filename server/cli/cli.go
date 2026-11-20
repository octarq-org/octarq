package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type Deps struct {
	Name             string
	PrintVersion     func(w io.Writer)
	Boot             func(ctx context.Context) int
	MCP              func(ctx context.Context) int
	OpenAPI          func(w io.Writer) int
	Backup           func(args []string) int
	Restore          func(args []string) int
	Plugin           func(args []string) int
	ApplyServerFlags func(port, host, configPath string) error
	ServiceName      string
	ServiceDisplay   string
	ServiceDesc      string
	Extra            func(root *cobra.Command)
}

type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func codeErr(code int) error {
	if code == 0 {
		return nil
	}
	return &exitError{code: code}
}

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

func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, d Deps) int {
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
