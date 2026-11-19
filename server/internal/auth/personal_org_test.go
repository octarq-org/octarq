package auth

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

// TestPersonalOrgIDResolvesOnlyPersonalOrgs proves the resolver never mistakes
// a team workspace for the personal one, even when the team org was created
// first and would win a plain "lowest org id" lookup.
func TestPersonalOrgIDResolvesOnlyPersonalOrgs(t *testing.T) {
	db := identityDB(t)
	user := mustUser(t, db, "solo@acme.com")

	team := mustOrg(t, db, "Team")
	if err := db.Create(&models.OrgMember{OrgID: team.ID, UserID: user.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("team membership: %v", err)
	}

	personal := mustOrg(t, db, "Personal")
	if err := db.Model(&models.Org{}).Where("id = ?", personal.ID).Update("kind", models.OrgKindPersonal).Error; err != nil {
		t.Fatalf("mark personal: %v", err)
	}
	if err := db.Create(&models.OrgMember{OrgID: personal.ID, UserID: user.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("personal membership: %v", err)
	}

	if got := PersonalOrgID(db, user.ID); got != personal.ID {
		t.Fatalf("PersonalOrgID = %d, want personal org %d (team is %d)", got, personal.ID, team.ID)
	}
}

// TestPersonalOrgIDFailsClosed covers every "cannot answer" input: a team-only
// member, an unknown user, a zero user, and a nil db all yield 0 so callers
// refuse instead of writing personal data into the active workspace.
func TestPersonalOrgIDFailsClosed(t *testing.T) {
	db := identityDB(t)
	user := mustUser(t, db, "team-only@acme.com")
	team := mustOrg(t, db, "Team")
	if err := db.Create(&models.OrgMember{OrgID: team.ID, UserID: user.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("membership: %v", err)
	}

	if got := PersonalOrgID(db, user.ID); got != 0 {
		t.Fatalf("team-only member resolved to %d, want 0", got)
	}
	if got := PersonalOrgID(db, 424242); got != 0 {
		t.Fatalf("unknown user resolved to %d, want 0", got)
	}
	if got := PersonalOrgID(db, 0); got != 0 {
		t.Fatalf("zero user resolved to %d, want 0", got)
	}
	if got := PersonalOrgID(nil, user.ID); got != 0 {
		t.Fatalf("nil db resolved to %d, want 0", got)
	}
}

// TestSignupProvisionsPersonalOrg pins the provisioning side of the contract:
// the org UpsertUserByEmail creates carries kind=personal, so PersonalOrgID can
// find it without guessing from ids.
func TestSignupProvisionsPersonalOrg(t *testing.T) {
	db := identityDB(t)
	_, orgID, err := UpsertUserByEmail(db, "fresh@acme.com", true)
	if err != nil {
		t.Fatalf("UpsertUserByEmail: %v", err)
	}
	var org models.Org
	if err := db.First(&org, orgID).Error; err != nil {
		t.Fatalf("load org: %v", err)
	}
	if org.Kind != models.OrgKindPersonal {
		t.Fatalf("signup org kind = %q, want %q", org.Kind, models.OrgKindPersonal)
	}
	if got := PersonalOrgID(db, mustUserID(t, db, "fresh@acme.com")); got != orgID {
		t.Fatalf("PersonalOrgID = %d, want signup org %d", got, orgID)
	}
}

func mustUserID(t *testing.T, db *gorm.DB, email string) uint {
	t.Helper()
	var u models.User
	if err := db.Where("email = ?", email).First(&u).Error; err != nil {
		t.Fatalf("load user %s: %v", email, err)
	}
	return u.ID
}
