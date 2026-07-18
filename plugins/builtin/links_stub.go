//go:build octarq_nolinks

package builtin

import (
	"github.com/octarq-org/octarq/plugin"
)

// Links returns nil when octarq_nolinks build tag is active.
func Links() plugin.Plugin {
	return nil
}
