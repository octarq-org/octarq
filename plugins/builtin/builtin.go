// Package builtin is the OSS edition's default Core feature set. It is one
// composition root's worth of plugins — the backend analog of the frontend's
// web/octarq.plugins.json default manifest.
//
// Composition is opt-in and uniform with Pro plugins: nothing auto-mounts inside
// app.New(); each entry point (octarq/main.go, octarq-pro's main, or a trimmed
// edition's own main) calls a.Use(...) for the plugins it wants. A trimmed
// edition builds a composition root that Uses a subset and simply does not import
// the excluded plugin packages — Go's linker then drops them from the binary, so
// no build tags are needed to exclude a feature.
package builtin

import (
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/plugins/links"
	"github.com/octarq-org/octarq/plugins/mail"
)

// Default returns the OSS Core feature plugins in dependency order (dns before
// links before mail, matching their Requires). Callers mount them via a.Use.
func Default() []plugin.Plugin {
	return []plugin.Plugin{dns.New(), links.New(), mail.New()}
}
