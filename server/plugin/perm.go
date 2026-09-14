package plugin

import (
	"net/http"
	"sync"
)

// Perm describes one gated operation.
type Perm struct {
	Key      string `json:"key"`      // "dns.records.delete" — module.resource.action, lowercase
	Module   string `json:"module"`   // "dns"
	Resource string `json:"resource"` // "DNS Record" — human label
	Action   string `json:"action"`   // create | read | update | delete | export
	Label    string `json:"label"`    // one line, shown in the roles matrix UI
	Default  string `json:"default"`  // built-in role required when nothing else decides
}

// PermResolver answers a permission question for one request. decided=false
// means "no opinion" — the caller falls back to the built-in role comparison.
type PermResolver func(r *http.Request, permKey string) (allow, decided bool)

var (
	permMu       sync.RWMutex
	permRegistry []Perm
	permResolver PermResolver
)

// DeclarePerm registers permission definitions (called during Mount).
//
// Keyed, not appended: Mount runs more than once per process — internal/mcp
// re-Mounts every plugin to build the MCP tool set — so appending would list
// each permission twice in the roles matrix, and once more for every future
// composition path. Re-declaring a key replaces it. Guarded by
// TestDeclarePermIsIdempotent.
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

// DeclaredPerms returns a copy of all registered permission definitions.
func DeclaredPerms() []Perm {
	permMu.RLock()
	defer permMu.RUnlock()
	out := make([]Perm, len(permRegistry))
	copy(out, permRegistry)
	return out
}

// SetPermResolver configures the global permission resolver.
func SetPermResolver(fn PermResolver) {
	permMu.Lock()
	defer permMu.Unlock()
	permResolver = fn
}

// ResolvePerm queries the registered PermResolver if present.
func ResolvePerm(r *http.Request, permKey string) (allow, decided bool) {
	permMu.RLock()
	fn := permResolver
	permMu.RUnlock()
	if fn == nil {
		return false, false
	}
	return fn(r, permKey)
}

// ResetPermRegistry clears declared permissions and the global resolver (useful for testing).
func ResetPermRegistry() {
	permMu.Lock()
	defer permMu.Unlock()
	permRegistry = nil
	permResolver = nil
}

// HasPerm is the nil-safe way for a plugin to ask Context.RequirePerm.
//
// It fails closed. The inverted form — `if c.RequirePerm != nil && !c.RequirePerm(...)`
// — reads as defensive and is the opposite: an unwired host leaves the field
// nil, the check is skipped entirely, and the endpoint opens to everyone. That
// exact bug shipped once in the links plugin; see plugins/links/role_gate_test.go.
func (c *Context) HasPerm(r *http.Request, permKey, minRole string) bool {
	if c == nil || c.RequirePerm == nil {
		return false
	}
	return c.RequirePerm(r, permKey, minRole)
}
