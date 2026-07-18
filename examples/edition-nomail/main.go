// Command edition-nomail is a worked example of a trimmed octarq edition: it
// composes only the dns and links Core plugins (no mail) by building its own
// composition root instead of using plugins/builtin.Default().
//
// Because this main never imports github.com/octarq-org/octarq/plugins/mail, the
// Go linker drops that package entirely — the mail feature is excluded from the
// binary with no build tags. CI proves this with `go tool nm` (see ci.yml).
//
// links requires dns (plugin.Info.Requires), so a "links-only" edition still
// mounts dns; app.preflightDependencies would refuse to start otherwise.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/octarq-org/octarq/app"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/plugins/links"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	a, err := app.New()
	if err != nil {
		slog.Error("init failed", "err", err)
		os.Exit(1)
	}
	// Composition root: only the wanted Core plugins. mail is not imported, so
	// the linker excludes it.
	a.Use(dns.New())
	a.Use(links.New())

	if err := a.Run(context.Background()); err != nil {
		slog.Error("run failed", "err", err)
		os.Exit(1)
	}
}
