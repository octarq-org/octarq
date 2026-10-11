# Engineering Specification — internal/authz

## 1. Overview
`internal/authz` defines the foundational workspace role types and hierarchical comparison logic for Octarq Core.

## 2. Roles & Hierarchy
- `RoleMember` (`"member"`): Rank 1. Standard team member with read and limited self-service permissions.
- `RoleAdmin` (`"admin"`): Rank 2. Administrator with settings management and team management privileges.
- `RoleOwner` (`"owner"`): Rank 3. Organization/workspace owner with destructive operations and billing authority.

## 3. Invariants & Fail-Closed Behavior
- **Explicit Rank Evaluation**: Any role string outside the canonical set (`member`, `admin`, `owner`), including empty strings or uppercase variants, receives rank 0.
- **Fail-Closed Guard**: `AtLeast(role, min)` returns `false` whenever either `role` or `min` has rank 0.

## 4. Verification
- Validated via unit tests and `authz_characterization_test.go`.
