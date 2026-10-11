package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

// OS system service lifecycle management.
//
// This file groups the kardianos/service integration, adapting the blocking
// server boot loop into the OS service supervisor lifecycle (systemd / Windows
// Services / macOS launchd) and handling install/start/stop/restart/status subcommands.

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
