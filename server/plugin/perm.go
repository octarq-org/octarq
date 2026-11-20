package plugin

import (
	"net/http"
	"sync"
)

type Perm struct {
	Key      string `json:"key"`
	Module   string `json:"module"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Label    string `json:"label"`
	Default  string `json:"default"`
}

type PermResolver func(r *http.Request, permKey string) (allow, decided bool)

var (
	permMu       sync.RWMutex
	permRegistry []Perm
	permResolver PermResolver
)

func DeclarePerm(perms ...Perm) {
	permMu.Lock()
	defer permMu.Unlock()
	for _, p := range perms {
		replaced := false
		for i := range permRegistry {
			if permRegistry[i].Key == p.Key {
				permRegistry[i] = p
				replaced = true
				break
			}
		}
		if !replaced {
			permRegistry = append(permRegistry, p)
		}
	}
}

func DeclaredPerms() []Perm {
	permMu.RLock()
	defer permMu.RUnlock()
	out := make([]Perm, len(permRegistry))
	copy(out, permRegistry)
	return out
}

func SetPermResolver(fn PermResolver) {
	permMu.Lock()
	defer permMu.Unlock()
	permResolver = fn
}

func ResolvePerm(r *http.Request, permKey string) (allow, decided bool) {
	permMu.RLock()
	fn := permResolver
	permMu.RUnlock()
	if fn == nil {
		return false, false
	}
	return fn(r, permKey)
}

func ResetPermRegistry() {
	permMu.Lock()
	defer permMu.Unlock()
	permRegistry = nil
	permResolver = nil
}

func (c *Context) HasPerm(r *http.Request, permKey, minRole string) bool {
	if c == nil || c.Host == nil || c.Host.Session() == nil {
		return false
	}
	return c.Host.Session().RequirePerm(r, permKey, minRole)
}
