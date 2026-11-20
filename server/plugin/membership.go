package plugin

import (
	"log"
	"sync"
)

type MemberRemovedHook func(orgID, userID uint)

var (
	memberRemovedMu    sync.RWMutex
	memberRemovedHooks map[string]MemberRemovedHook
)

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

func ResetMemberRemovedHooks() {
	memberRemovedMu.Lock()
	defer memberRemovedMu.Unlock()
	memberRemovedHooks = nil
}
