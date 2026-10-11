package authz_test

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/authz"
)

func TestAuthzCharacterizationRoles(t *testing.T) {
	if authz.RoleMember != "member" {
		t.Fatalf("RoleMember mismatch: %q", authz.RoleMember)
	}
	if authz.RoleAdmin != "admin" {
		t.Fatalf("RoleAdmin mismatch: %q", authz.RoleAdmin)
	}
	if authz.RoleOwner != "owner" {
		t.Fatalf("RoleOwner mismatch: %q", authz.RoleOwner)
	}
}

func TestAuthzCharacterizationFailClosed(t *testing.T) {
	// Any non-canonical roles must strictly fail closed
	badRoles := []authz.Role{"", "root", "system", "anonymous", "null", "OWNER", "Admin"}
	for _, r := range badRoles {
		if authz.AtLeast(r, authz.RoleMember) {
			t.Fatalf("expected role %q to fail closed against RoleMember", r)
		}
		if authz.AtLeast(authz.RoleOwner, r) {
			t.Fatalf("expected RoleOwner against invalid min %q to fail closed", r)
		}
	}
}
