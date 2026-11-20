package authz

type Role string

const (
	RoleMember Role = "member"
	RoleAdmin  Role = "admin"
	RoleOwner  Role = "owner"
)

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

func AtLeast(role, min Role) bool {
	rRank := roleRank(role)
	mRank := roleRank(min)
	if rRank == 0 || mRank == 0 {
		return false
	}
	return rRank >= mRank
}
