package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/octarq-org/octarq/server/plugin"
)

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
