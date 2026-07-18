//go:build !octarq_nodns

package builtin

import (
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
)

// DNS returns the default-on dns plugin.
func DNS() plugin.Plugin {
	return dns.New()
}
