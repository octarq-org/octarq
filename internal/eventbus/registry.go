package eventbus

import "sync"

// EventDef describes a webhook event subscribers can select in the dashboard.
// Core events are registered in this package's init; plugins contribute theirs
// during Mount through the plugin.Context.RegisterWebhookEvent seam, so the
// dashboard only offers events that can actually fire in this build.
type EventDef struct {
	Key         string `json:"key"`         // e.g. "link.click"
	Group       string `json:"group"`       // e.g. "Links"
	Title       string `json:"title"`       // e.g. "Link Clicked"
	Description string `json:"description"` // when the event fires
}

// EventGroup is one named group of event definitions, in registration order.
type EventGroup struct {
	Group  string     `json:"group"`
	Events []EventDef `json:"events"`
}

var (
	defsMu     sync.RWMutex
	defsByKey  = map[string]struct{}{}
	groupOrder []string
	defsByGrp  = map[string][]EventDef{}
)

// RegisterEventDef adds an event definition to the registry. Registering the
// same Key twice is a no-op, so a plugin mounted for both HTTP and MCP serving
// contributes each event once.
func RegisterEventDef(def EventDef) {
	if def.Key == "" {
		return
	}
	defsMu.Lock()
	defer defsMu.Unlock()
	if _, dup := defsByKey[def.Key]; dup {
		return
	}
	defsByKey[def.Key] = struct{}{}
	if _, seen := defsByGrp[def.Group]; !seen {
		groupOrder = append(groupOrder, def.Group)
	}
	defsByGrp[def.Group] = append(defsByGrp[def.Group], def)
}

// EventGroups returns all registered event definitions grouped in registration
// order — deterministic so the dashboard renders a stable list.
func EventGroups() []EventGroup {
	defsMu.RLock()
	defer defsMu.RUnlock()
	out := make([]EventGroup, 0, len(groupOrder))
	for _, g := range groupOrder {
		out = append(out, EventGroup{Group: g, Events: append([]EventDef(nil), defsByGrp[g]...)})
	}
	return out
}

// Core events fired by octarq itself (workspace membership, authentication).
// Feature events (links, mail, domains) are registered by their plugins.
func init() {
	for _, d := range []EventDef{
		{Key: "member.invite", Group: "Member", Title: "Member Invited", Description: "A user was invited to the workspace"},
		{Key: "member.join", Group: "Member", Title: "Member Joined", Description: "An invited user accepted and joined the workspace"},
		{Key: "member.remove", Group: "Member", Title: "Member Removed", Description: "A member was removed from the workspace"},
		{Key: "auth.login_failed", Group: "Auth", Title: "Login Failed", Description: "A sign-in attempt failed for a workspace account"},
		{Key: "auth.password_changed", Group: "Auth", Title: "Password Changed", Description: "An account password was changed"},
	} {
		RegisterEventDef(d)
	}
}
