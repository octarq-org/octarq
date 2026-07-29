package auth

import "testing"

// A plugin registers its auth method at Mount — before any configuration
// exists — and there is no Unregister. So an unconfigured method was offered
// on the login page from the moment its plugin was compiled in, and clicking
// it reached a handler that refused. List has to do the filtering.
func TestListOmitsUnavailableMethods(t *testing.T) {
	ResetMethodsForTesting()
	t.Cleanup(ResetMethodsForTesting)

	configured := false
	Register(AuthMethod{ID: "sso", Label: "Sign in with SSO", Available: func() bool { return configured }})
	Register(AuthMethod{ID: "always", Label: "Always"})

	ids := func() []string {
		var out []string
		for _, m := range List() {
			out = append(out, m.ID)
		}
		return out
	}

	got := ids()
	if len(got) != 1 || got[0] != "always" {
		t.Fatalf("unconfigured method offered: %v", got)
	}

	// Consulted per call, so configuring it later makes it appear without
	// re-registration — which is the case Mount-time registration cannot serve.
	configured = true
	if len(ids()) != 2 {
		t.Fatalf("configured method still hidden: %v", ids())
	}

	// A nil predicate must keep meaning "always", or every existing plugin
	// silently vanishes from the login page.
	configured = false
	for _, m := range List() {
		if m.ID == "always" {
			return
		}
	}
	t.Fatal("method with no Available predicate was filtered out")
}
