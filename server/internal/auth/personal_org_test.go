package auth

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

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

func TestEnsurePersonalOrgCreatesForExistingTeamMember(t *testing.T) {
	db := identityDB(t)
	user := mustUser(t, db, "teammate@acme.com")
	team := mustOrg(t, db, "Team")
	if err := db.Create(&models.OrgMember{OrgID: team.ID, UserID: user.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("team membership: %v", err)
	}

	first, err := EnsurePersonalOrg(db, user.ID)
	if err != nil {
		t.Fatalf("EnsurePersonalOrg: %v", err)
	}
	if first == 0 || first == team.ID {
		t.Fatalf("personal org = %d, want a new org distinct from team %d", first, team.ID)
	}
	var org models.Org
	if err := db.First(&org, first).Error; err != nil {
		t.Fatalf("load org: %v", err)
	}
	if org.Kind != models.OrgKindPersonal {
		t.Fatalf("kind = %q, want %q", org.Kind, models.OrgKindPersonal)
	}
	if got := PersonalOrgID(db, user.ID); got != first {
		t.Fatalf("PersonalOrgID = %d, want %d", got, first)
	}

	second, err := EnsurePersonalOrg(db, user.ID)
	if err != nil {
		t.Fatalf("EnsurePersonalOrg second: %v", err)
	}
	if second != first {
		t.Fatalf("second call returned %d, want the existing %d", second, first)
	}
}

func TestBackfillPersonalOrgsProvisionsEveryUser(t *testing.T) {
	db := identityDB(t)
	withTeam := mustUser(t, db, "one@acme.com")
	team := mustOrg(t, db, "Team")
	if err := db.Create(&models.OrgMember{OrgID: team.ID, UserID: withTeam.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("membership: %v", err)
	}
	solo := mustUser(t, db, "two@acme.com")
	if err := db.Create(&models.OrgMember{OrgID: team.ID, UserID: solo.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("membership: %v", err)
	}

	if err := BackfillPersonalOrgs(db); err != nil {
		t.Fatalf("BackfillPersonalOrgs: %v", err)
	}
	for _, uid := range []uint{withTeam.ID, solo.ID} {
		if got := PersonalOrgID(db, uid); got == 0 {
			t.Fatalf("user %d has no personal org after backfill", uid)
		}
	}
	if err := BackfillPersonalOrgs(db); err != nil {
		t.Fatalf("BackfillPersonalOrgs second: %v", err)
	}
	var count int64
	if err := db.Table("orgs").Where("kind = ?", models.OrgKindPersonal).Count(&count).Error; err != nil {
		t.Fatalf("count personal orgs: %v", err)
	}
	if count != 2 {
		t.Fatalf("personal orgs = %d, want 2", count)
	}
}
