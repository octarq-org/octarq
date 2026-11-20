package plugin

import (
	"log"
	"strings"
)

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
	_, stopper := p.(Stopper)
	add("stop", stopper)
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
