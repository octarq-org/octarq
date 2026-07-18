//go:build !octarq_nolinks

package builtin

import (
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/links"
)

// Links returns the default-on links plugin.
func Links() plugin.Plugin {
	return links.New()
}
