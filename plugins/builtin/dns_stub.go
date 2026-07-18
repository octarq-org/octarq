//go:build octarq_nodns

package builtin

import (
	"github.com/octarq-org/octarq/plugin"
)

// DNS returns nil when octarq_nodns build tag is active.
func DNS() plugin.Plugin {
	return nil
}
