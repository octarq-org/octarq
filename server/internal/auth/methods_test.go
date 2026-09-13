package auth

import (
	"testing"
)

func TestAuthMethodsRegistry(t *testing.T) {
	ResetMethodsForTesting()
	defer ResetMethodsForTesting()

	list := List()
	if list == nil || len(list) != 0 {
		t.Fatalf("expected empty non-nil slice, got %#v", list)
	}

	m1 := AuthMethod{
		ID:       "sso",
		Label:    "Sign in with SSO",
		LoginURL: "/api/sso/login",
		IconKey:  "shield",
	}

	Register(m1)

	list = List()
	if len(list) != 1 {
		t.Fatalf("expected 1 method, got %d", len(list))
	}
	if list[0].ID != "sso" || list[0].Label != "Sign in with SSO" {
		t.Fatalf("unexpected method: %#v", list[0])
	}

	// Idempotent overwrite
	m1Updated := AuthMethod{
		ID:       "sso",
		Label:    "Sign in with Enterprise SSO",
		LoginURL: "/api/sso/login",
		IconKey:  "shield-key",
	}
	Register(m1Updated)

	list = List()
	if len(list) != 1 {
		t.Fatalf("expected 1 method after overwrite, got %d", len(list))
	}
	if list[0].Label != "Sign in with Enterprise SSO" {
		t.Fatalf("expected updated label, got %s", list[0].Label)
	}
}
