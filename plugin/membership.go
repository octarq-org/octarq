package plugin

import (
	"log"
	"sync"
)

// MemberRemovedHook is notified when a member is removed from an organization.
//
// This is a ONE-WAY, BEST-EFFORT cleanup notification — not an authorization
// mechanism. By the time a hook runs, the member is already gone: core has
// deleted the org_members row and revoked their sessions and API tokens. The
// hook exists so a plugin can tidy its own per-member state (role assignments,
// cached identity, derived rows) that would otherwise outlive the membership
// and silently resurrect when the same person is invited back.
//
// The contract is intentionally weak: no ordering guarantee, no delivery
// guarantee (a process restart between removal and notification loses the
// event, and a hook is free to do nothing). A plugin MUST NOT base an
// authorization decision on having — or not having — received this hook.
// Authorization must live-read org_members on every request, exactly like
// the resolver in octarq-pro#305 does; this hook is for cleaning up state,
// never for deciding who may act.
type MemberRemovedHook func(orgID, userID uint)

var (
	memberRemovedMu    sync.RWMutex
	memberRemovedHooks map[string]MemberRemovedHook
)

// RegisterMemberRemovedHook registers a MemberRemovedHook under a plugin name.
//
// Keyed, not appended: Mount runs more than once per process — internal/mcp
// re-Mounts every plugin to build the MCP tool set — so appending would leave
// one stale copy of the hook per re-Mount, and the cleanup logic would run N
// times (with N growing with process lifetime). Re-registering a name
// replaces the previous hook, mirroring DeclarePerm.
func RegisterMemberRemovedHook(name string, hook MemberRemovedHook) {
	if hook == nil {
		return
	}
	memberRemovedMu.Lock()
	defer memberRemovedMu.Unlock()
	if memberRemovedHooks == nil {
		memberRemovedHooks = make(map[string]MemberRemovedHook)
	}
	memberRemovedHooks[name] = hook
}

// NotifyMemberRemoved invokes every registered MemberRemovedHook with the
// removed member's (orgID, userID).
//
// Call it only AFTER the membership row is actually deleted: a hook that fires
// for a still-present member would wipe state for someone who is still in the
// workspace. Hooks run in no particular order. Each hook is best-effort — a
// panic inside one is recovered and logged so it can never take down the
// removal request.
//
// This is a cleanup notification, not an authorization signal: it can be lost
// (process restart), so plugins must live-read org_members on every request
// instead of trusting this callback. See MemberRemovedHook.
func NotifyMemberRemoved(orgID, userID uint) {
	memberRemovedMu.RLock()
	hooks := make([]MemberRemovedHook, 0, len(memberRemovedHooks))
	for _, h := range memberRemovedHooks {
		hooks = append(hooks, h)
	}
	memberRemovedMu.RUnlock()

	for _, h := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[plugin] member-removed hook panicked (org=%d user=%d): %v", orgID, userID, r)
				}
			}()
			h(orgID, userID)
		}()
	}
}

// ResetMemberRemovedHooks clears all registered member-removal hooks (useful
// for testing), mirroring ResetPermRegistry.
func ResetMemberRemovedHooks() {
	memberRemovedMu.Lock()
	defer memberRemovedMu.Unlock()
	memberRemovedHooks = nil
}
