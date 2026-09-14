package authz

// Role is a workspace role, ordered by privilege.
type Role string

const (
	RoleMember Role = "member"
	RoleAdmin  Role = "admin"
	RoleOwner  Role = "owner"
)

// roleRank assigns a numeric rank to valid roles so privilege comparison is explicit.
// Unknown or empty roles have rank 0.
func roleRank(r Role) int {
	switch r {
	case RoleMember:
		return 1
	case RoleAdmin:
		return 2
	case RoleOwner:
		return 3
	default:
		return 0
	}
}

// AtLeast reports whether role meets the minimum privilege level.
// Unknown roles or empty strings return false (fail closed).
func AtLeast(role, min Role) bool {
	rRank := roleRank(role)
	mRank := roleRank(min)
	if rRank == 0 || mRank == 0 {
		return false
	}
	return rRank >= mRank
}
