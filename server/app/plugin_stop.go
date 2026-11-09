package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/octarq-org/octarq/server/plugin"
)

// stopPlugins cancels the context every plugin Start goroutine was given, then
// calls Stop on each plugin that implements plugin.Stopper, in the reverse of
// the order the plugins were mounted.
//
// Reverse order is the point of the whole exercise: a plugin mounted later may
// drain by writing through a service provided by a plugin mounted earlier, so
// the earlier one has to still be alive while the later one stops. Running them
// in mount order instead would tear the provider down first and leave the
// consumer writing into a closed service.
//
// Stop runs on one goroutine. The process is going down and ctx carries the
// shutdown deadline; a plugin that can usefully parallelize its own drain does
// so inside its Stop.
//
// Errors are collected rather than short-circuited: one plugin failing to drain
// must not skip every plugin after it. Stop is skipped for a plugin whose Start
// never ran only in the sense that a plugin implementing Stopper is always
// called — implementations must tolerate a Start that never ran.
func stopPlugins(ctx context.Context, stopContext context.CancelFunc, plugins []plugin.Plugin) error {
	if stopContext != nil {
		stopContext()
	}
	var errs []error
	for i := len(plugins) - 1; i >= 0; i-- {
		p := plugins[i]
		stopper, ok := p.(plugin.Stopper)
		if !ok {
			continue
		}
		if err := stopper.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("plugin %q shutdown failed: %w", p.Name(), err))
			continue
		}
		slog.Debug("plugin stopped", "name", p.Name())
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
