package authz

import (
	"testing"
)

func TestAtLeast(t *testing.T) {
	tests := []struct {
		role Role
		min  Role
		want bool
	}{
		{RoleOwner, RoleOwner, true},
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleMember, true},

		{RoleAdmin, RoleOwner, false},
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleMember, true},

		{RoleMember, RoleOwner, false},
		{RoleMember, RoleAdmin, false},
		{RoleMember, RoleMember, true},

		{"", RoleMember, false},
		{"", RoleAdmin, false},
		{"", RoleOwner, false},
		{"guest", RoleMember, false},
		{"superadmin", RoleAdmin, false},
		{RoleOwner, "invalid", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role)+"_vs_"+string(tt.min), func(t *testing.T) {
			if got := AtLeast(tt.role, tt.min); got != tt.want {
				t.Errorf("AtLeast(%q, %q) = %v, want %v", tt.role, tt.min, got, tt.want)
			}
		})
	}
}
