package auth

import "sync"

// AuthMethod is a provider-agnostic auth method definition for third-party / SSO login ways.
type AuthMethod struct {
	ID       string `json:"id"`       // stable identifier, e.g. "sso" / "oidc-acme"
	Label    string `json:"label"`    // button label, e.g. "Sign in with SSO"
	LoginURL string `json:"loginUrl"` // launch/redirect URL (plugin's /api/... endpoint)
	IconKey  string `json:"iconKey"`  // frontend icon key, optional
}

var (
	methodsMu sync.RWMutex
	methods   = make(map[string]AuthMethod)
)

// Register adds or updates an AuthMethod in the registry by ID. It is idempotent
// and safe for concurrent use.
func Register(m AuthMethod) {
	if m.ID == "" {
		return
	}
	methodsMu.Lock()
	defer methodsMu.Unlock()
	methods[m.ID] = m
}

// List returns all registered AuthMethods. Returns an empty non-nil slice if none registered.
func List() []AuthMethod {
	methodsMu.RLock()
	defer methodsMu.RUnlock()

	result := make([]AuthMethod, 0, len(methods))
	for _, m := range methods {
		result = append(result, m)
	}
	return result
}

// ResetMethodsForTesting clears the registry for tests.
func ResetMethodsForTesting() {
	methodsMu.Lock()
	defer methodsMu.Unlock()
	methods = make(map[string]AuthMethod)
}
