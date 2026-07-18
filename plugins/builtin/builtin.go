// Package builtin provides the list of default-on Core plugins (dns, mail, links).
// Individual plugins can be excluded at compile time using build tags:
// octarq_nodns, octarq_nomail, octarq_nolinks.
package builtin

import (
	"github.com/octarq-org/octarq/plugin"
)

// All returns the slice of Core plugins enabled for this build.
func All() []plugin.Plugin {
	var plugins []plugin.Plugin
	if dns := DNS(); dns != nil {
		plugins = append(plugins, dns)
	}
	if mail := Mail(); mail != nil {
		plugins = append(plugins, mail)
	}
	if links := Links(); links != nil {
		plugins = append(plugins, links)
	}
	return plugins
}
