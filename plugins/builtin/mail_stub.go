//go:build octarq_nomail

package builtin

import (
	"github.com/octarq-org/octarq/plugin"
)

// Mail returns nil when octarq_nomail build tag is active.
func Mail() plugin.Plugin {
	return nil
}
