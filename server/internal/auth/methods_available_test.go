package auth

import "testing"

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

	configured = true
	if len(ids()) != 2 {
		t.Fatalf("configured method still hidden: %v", ids())
	}

	configured = false
	for _, m := range List() {
		if m.ID == "always" {
			return
		}
	}
	t.Fatal("method with no Available predicate was filtered out")
}
