//go:build !octarq_nomail

package builtin

import (
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/mail"
)

// Mail returns the default-on mail plugin.
func Mail() plugin.Plugin {
	return mail.New()
}
