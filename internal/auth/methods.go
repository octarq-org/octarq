package auth

import "sync"

// AuthMethod is a provider-agnostic auth method definition for third-party / SSO login ways.
type AuthMethod struct {
	ID       string `json:"id"`       // stable identifier, e.g. "sso" / "oidc-acme"
	Label    string `json:"label"`    // button label, e.g. "Sign in with SSO"
	LoginURL string `json:"loginUrl"` // launch/redirect URL (plugin's /api/... endpoint)
	IconKey  string `json:"iconKey"`  // frontend icon key, optional
	// Available gates whether List returns this method; see
	// plugin.AuthMethod.Available. nil means always.
	Available func() bool `json:"-"`
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

// List returns the AuthMethods that are usable right now. Returns an empty
// non-nil slice if none are.
//
// Registration is permanent — it happens at Mount and there is no Unregister —
// so filtering here, per call, is what lets a method appear and disappear with
// its configuration. Offering an unusable method is not a cosmetic problem: the
// login page renders whatever this returns, and a method that cannot serve a
// login sends the user to an error page.
func List() []AuthMethod {
	methodsMu.RLock()
	defer methodsMu.RUnlock()

	result := make([]AuthMethod, 0, len(methods))
	for _, m := range methods {
		if m.Available != nil && !m.Available() {
			continue
		}
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
