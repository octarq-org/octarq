package plugin

import (
	"log"
	"strings"
)

// DetectedCapabilities reports, in a stable order, the names of the optional
// interfaces p was actually detected as implementing.
//
// It exists because optional-capability detection is invisible when it fails.
// The app opts a plugin into extra behavior with a runtime type assertion, so a
// method named Menu() instead of Menus(), or a Start(context.Context) that
// drifted to Start(context.Context) error, produces a plugin that mounts
// cleanly, logs nothing, and simply never gets that capability invoked. Core
// plugins are protected by the compile-time assertions the package doc mandates
// (`var _ plugin.MenuProvider = (*Plugin)(nil)`), but a third-party author who
// skipped them gets no signal at all — no menu, no error, nothing to search for.
//
// This does not make the typo fail; nothing at runtime can. It makes the result
// observable: an author who expected "menu" in this list and does not see it has
// the answer immediately instead of reading the mount path looking for a bug
// that is in their own method name.
//
// The returned names are the stable capability keys ("start", "menu",
// "instance-menu", "action", "mcp", "help", "openapi", "describe"), suitable for
// a log line or an API field.
func DetectedCapabilities(p Plugin) []string {
	if p == nil {
		return nil
	}
	var caps []string
	add := func(name string, ok bool) {
		if ok {
			caps = append(caps, name)
		}
	}
	_, starter := p.(Starter)
	add("start", starter)
	_, menu := p.(MenuProvider)
	add("menu", menu)
	_, instanceMenu := p.(InstanceMenuProvider)
	add("instance-menu", instanceMenu)
	_, action := p.(ActionProvider)
	add("action", action)
	_, mcp := p.(MCPProvider)
	add("mcp", mcp)
	_, help := p.(HelpProvider)
	add("help", help)
	_, openapi := p.(OpenAPIContributor)
	add("openapi", openapi)
	_, describer := p.(Describer)
	add("describe", describer)
	return caps
}

// LogDetectedCapabilities writes one line per plugin naming the optional
// capabilities it was detected as implementing, so a capability that silently
// failed to be detected is visible in the startup log rather than only in its
// absence from the UI. Call it once per plugin from the mount loop.
func LogDetectedCapabilities(p Plugin) {
	if p == nil {
		return
	}
	caps := DetectedCapabilities(p)
	if len(caps) == 0 {
		log.Printf("plugin %s: optional capabilities: none", p.Name())
		return
	}
	log.Printf("plugin %s: optional capabilities: %s", p.Name(), strings.Join(caps, ", "))
}
