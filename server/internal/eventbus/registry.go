package eventbus

import "sync"

type EventDef struct {
	Key         string `json:"key"`
	Group       string `json:"group"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

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

func EventGroups() []EventGroup {
	defsMu.RLock()
	defer defsMu.RUnlock()
	out := make([]EventGroup, 0, len(groupOrder))
	for _, g := range groupOrder {
		out = append(out, EventGroup{Group: g, Events: append([]EventDef(nil), defsByGrp[g]...)})
	}
	return out
}

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
