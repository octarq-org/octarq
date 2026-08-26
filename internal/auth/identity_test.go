package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// identityDB gives each test its own in-memory database. The package-wide
// testDB helper shares one (`file::memory:?cache=shared`), and these tests
// assert on row *counts* — "the refused login wrote nothing" is only a claim
// about this test's writes.
func identityDB(t *testing.T) *gorm.DB {
	t.Helper()
	// The name goes into a URI, so anything with meaning there has to go. A
	// subtest named "" becomes "#00", and the '#' truncated the DSN at the
	// fragment — dropping mode=memory, writing an actual file into the package
	// directory, and sharing it with the next case.
	safe := strings.Map(func(r rune) rune {
		if r == '#' || r == '?' || r == '/' || r == '&' || r == '=' || r == ' ' {
			return '-'
		}
		return r
	}, t.Name())
	dsn := fmt.Sprintf("file:identity-%s?mode=memory&cache=shared", safe)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func identityManager(t *testing.T, db *gorm.DB) *Manager {
	t.Helper()
	return New(&config.Config{SecretKey: "secret"}, crypto.New("secret")).WithDB(db)
}

// oidcID builds a verified-looking assertion for the tests below.
func oidcID(issuer, subject, email string) plugin.ExternalIdentity {
	return plugin.ExternalIdentity{
		Provider: plugin.ProviderOIDC,
		Issuer:   issuer,
		Subject:  subject,
		Email:    email,
	}
}

func mustUser(t *testing.T, db *gorm.DB, email string) models.User {
	t.Helper()
	u := models.User{Email: email, EmailVerified: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func mustOrg(t *testing.T, db *gorm.DB, name string) models.Org {
	t.Helper()
	slug, err := models.AllocateOrgSlug(db)
	if err != nil {
		t.Fatalf("allocate slug: %v", err)
	}
	o := models.Org{Name: name, Slug: slug}
	if err := db.Create(&o).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	return o
}

// Row 6 of the decision table, and the reason the table exists. Once each org
// supplies its own issuer, an org admin can stand up an IdP and assert any
// address they like. Matching that address to an existing account is handing
// the account over, so an unbound identity whose email is taken must be refused.
func TestLoginByIdentityRefusesUnboundIdentityForExistingEmail(t *testing.T) {
	db := identityDB(t)
	mustUser(t, db, "victim@acme.com")

	_, _, err := resolveIdentity(db, oidcID("https://evil.example", "attacker-sub", "victim@acme.com"))
	if !errors.Is(err, ErrAccountLinkRequired) {
		t.Fatalf("resolveIdentity = %v, want ErrAccountLinkRequired", err)
	}
	var n int64
	db.Model(&models.UserIdentity{}).Count(&n)
	if n != 0 {
		t.Fatalf("refused login still wrote %d binding(s)", n)
	}
}

// Row 5: the reverse chain, which is worse. If an unapproved IdP may mint an
// account for an address nobody has registered yet, the attacker pre-owns that
// address on this instance — and the workspace invitation that later goes to it
// attaches to *their* user row.
func TestLoginByIdentityRefusesAccountMintingWithoutApproval(t *testing.T) {
	db := identityDB(t)
	id := oidcID("https://evil.example", "attacker-sub", "victim@acme.com")
	id.MayCreateUser = false

	if _, _, err := resolveIdentity(db, id); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("resolveIdentity = %v, want ErrRegistrationDisabled", err)
	}
	var users int64
	db.Model(&models.User{}).Where("email = ?", "victim@acme.com").Count(&users)
	if users != 0 {
		t.Fatalf("refused login created %d user(s) for the asserted email", users)
	}
}

// Row 4: an approved IdP may create the account, bind it, and (with AllowJIT)
// place it in the org the login URL named.
func TestLoginByIdentityCreatesUserWhenApproved(t *testing.T) {
	db := identityDB(t)
	org := mustOrg(t, db, "Acme")

	id := oidcID("https://idp.acme.com/", "sub-1", "New@Acme.com")
	id.OrgID, id.AllowJIT, id.MayCreateUser = org.ID, true, true

	uid, orgID, err := resolveIdentity(db, id)
	if err != nil {
		t.Fatalf("resolveIdentity: %v", err)
	}
	if orgID != org.ID {
		t.Fatalf("orgID = %d, want %d", orgID, org.ID)
	}

	var user models.User
	if err := db.First(&user, uid).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.Email != "new@acme.com" {
		t.Errorf("email = %q, want it lowercased", user.Email)
	}
	// A JIT arrival is never an instance admin, whatever the IdP asserts.
	if user.IsInstanceAdmin {
		t.Error("JIT-provisioned user became an instance admin")
	}

	var bound models.UserIdentity
	if err := db.Where("user_id = ?", uid).First(&bound).Error; err != nil {
		t.Fatalf("binding not written: %v", err)
	}
	// Trailing slashes are normalized on the way in; storing both spellings of
	// an issuer silently unbinds everyone who signed in under the other one.
	if bound.Issuer != "https://idp.acme.com" {
		t.Errorf("stored issuer = %q, want it normalized", bound.Issuer)
	}

	var member models.OrgMember
	if err := db.Where("org_id = ? AND user_id = ?", org.ID, uid).First(&member).Error; err != nil {
		t.Fatalf("not joined to the org: %v", err)
	}
	if member.Role != "member" {
		t.Errorf("role = %q, want member by default", member.Role)
	}
}

// Row 3: a bound identity that is not a member of the org it arrived through
// does not get in when that org has JIT turned off.
func TestLoginByIdentityRefusesNonMemberWithoutJIT(t *testing.T) {
	db := identityDB(t)
	org := mustOrg(t, db, "Acme")
	user := mustUser(t, db, "bob@acme.com")
	id := oidcID("https://idp.acme.com", "sub-bob", "bob@acme.com")
	if err := bindIdentity(db, user.ID, id); err != nil {
		t.Fatalf("bind: %v", err)
	}

	id.OrgID, id.AllowJIT = org.ID, false
	if _, _, err := resolveIdentity(db, id); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("resolveIdentity = %v, want ErrRegistrationDisabled", err)
	}
	var n int64
	db.Model(&models.OrgMember{}).Where("org_id = ?", org.ID).Count(&n)
	if n != 0 {
		t.Fatalf("refused login still joined the org (%d members)", n)
	}
}

// Row 1, plus the invariant that arriving through one org's SSO changes nothing
// anywhere else: JIT membership touches the org named in the URL and no other.
func TestLoginByIdentityJITTouchesOnlyTheNamedOrg(t *testing.T) {
	db := identityDB(t)
	acme := mustOrg(t, db, "Acme")
	other := mustOrg(t, db, "Other")
	user := mustUser(t, db, "bob@acme.com")

	id := oidcID("https://idp.acme.com", "sub-bob", "bob@acme.com")
	if err := bindIdentity(db, user.ID, id); err != nil {
		t.Fatalf("bind: %v", err)
	}
	id.OrgID, id.AllowJIT, id.JITRole = acme.ID, true, "admin"

	uid, orgID, err := resolveIdentity(db, id)
	if err != nil {
		t.Fatalf("resolveIdentity: %v", err)
	}
	if uid != user.ID || orgID != acme.ID {
		t.Fatalf("resolved (%d, %d), want (%d, %d)", uid, orgID, user.ID, acme.ID)
	}
	var n int64
	db.Model(&models.OrgMember{}).Where("org_id = ?", other.ID).Count(&n)
	if n != 0 {
		t.Fatalf("SSO into Acme created %d membership(s) in the other org", n)
	}
	// A second sign-in resolves the same way rather than stacking memberships.
	if _, _, err := resolveIdentity(db, id); err != nil {
		t.Fatalf("second resolveIdentity: %v", err)
	}
	db.Model(&models.OrgMember{}).Where("org_id = ?", acme.ID).Count(&n)
	if n != 1 {
		t.Fatalf("membership count = %d after two sign-ins, want 1", n)
	}
}

// The issuer is half the key. Two IdPs may well hand out the same subject, so
// the same subject under a different issuer must resolve to nobody.
func TestIdentityKeyIsScopedToIssuer(t *testing.T) {
	db := identityDB(t)
	user := mustUser(t, db, "bob@acme.com")
	if err := bindIdentity(db, user.ID, oidcID("https://idp.acme.com", "sub-1", "bob@acme.com")); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// Same subject, attacker's issuer, same email → not a match, and the email
	// collision then refuses the sign-in outright.
	if _, _, err := resolveIdentity(db, oidcID("https://evil.example", "sub-1", "bob@acme.com")); !errors.Is(err, ErrAccountLinkRequired) {
		t.Fatalf("resolveIdentity = %v, want ErrAccountLinkRequired", err)
	}
}

// Binding is how row 6 is resolved, and it must not be a way to take over a
// binding somebody else already holds.
func TestBindIdentityRefusesToStealABinding(t *testing.T) {
	db := identityDB(t)
	alice := mustUser(t, db, "alice@acme.com")
	bob := mustUser(t, db, "bob@acme.com")
	id := oidcID("https://idp.acme.com", "sub-1", "alice@acme.com")

	if err := bindIdentity(db, alice.ID, id); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := bindIdentity(db, bob.ID, id); !errors.Is(err, ErrIdentityBoundElsewhere) {
		t.Fatalf("bind = %v, want ErrIdentityBoundElsewhere", err)
	}
	// Re-binding to the same user stays a no-op rather than a duplicate row.
	if err := bindIdentity(db, alice.ID, id); err != nil {
		t.Fatalf("rebind to owner: %v", err)
	}
	var n int64
	db.Model(&models.UserIdentity{}).Count(&n)
	if n != 1 {
		t.Fatalf("binding rows = %d, want 1", n)
	}
}

// BindIdentity is the authenticated half of the pair: no session, no binding.
func TestBindIdentityRequiresASession(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)
	req := httptest.NewRequest(http.MethodPost, "/", nil)

	if err := m.BindIdentity(req, oidcID("https://idp.acme.com", "sub-1", "a@b.c")); err == nil {
		t.Fatal("BindIdentity accepted an unauthenticated request")
		return
	}
	var n int64
	db.Model(&models.UserIdentity{}).Count(&n)
	if n != 0 {
		t.Fatalf("unauthenticated bind wrote %d row(s)", n)
	}
}

// An invite-only instance does not get JIT accounts through the side door,
// however the per-IdP approval is set — same policy LoginByEmail honours.
func TestLoginByIdentityHonoursInviteOnly(t *testing.T) {
	db := identityDB(t)
	if err := db.Create(&models.Setting{Key: "allow_registration", Value: "false"}).Error; err != nil {
		t.Fatalf("setting: %v", err)
	}
	id := oidcID("https://idp.acme.com", "sub-1", "new@acme.com")
	id.MayCreateUser = true

	if _, _, err := resolveIdentity(db, id); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("resolveIdentity = %v, want ErrRegistrationDisabled", err)
	}
}

// LoginByIdentity issues an ordinary session, so an SSO user is not a second
// class of logged-in.
func TestLoginByIdentityIssuesSession(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)
	org := mustOrg(t, db, "Acme")

	id := oidcID("https://idp.acme.com", "sub-1", "new@acme.com")
	id.OrgID, id.AllowJIT, id.MayCreateUser = org.ID, true, true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	uid, err := m.LoginByIdentity(rec, req, id)
	if err != nil {
		t.Fatalf("LoginByIdentity: %v", err)
	}
	if uid == 0 {
		t.Fatal("LoginByIdentity returned user 0")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("LoginByIdentity issued no session cookie")
	}
}

// An identity provider does not get to hand out the owner role. The org admin
// who configures that provider is refused the owner role through the members
// API — "self-promotion by proxy" — and asserting it in an ID token would be
// the same promotion by a longer road.
func TestJITRoleCannotReachOwner(t *testing.T) {
	for _, asserted := range []string{"owner", "OWNER", " owner ", "instance_admin", "superuser", ""} {
		// Subtests, so identityDB's per-test database is actually per case.
		t.Run(asserted, func(t *testing.T) {
			db := identityDB(t)
			org := mustOrg(t, db, "Acme")
			user := mustUser(t, db, "bob@acme.com")
			id := oidcID("https://idp.acme.com", "sub-bob", "bob@acme.com")
			if err := bindIdentity(db, user.ID, id); err != nil {
				t.Fatalf("bind: %v", err)
			}
			id.OrgID, id.AllowJIT, id.JITRole = org.ID, true, asserted

			if _, _, err := resolveIdentity(db, id); err != nil {
				t.Fatalf("resolveIdentity(%q): %v", asserted, err)
			}
			var member models.OrgMember
			if err := db.Where("org_id = ? AND user_id = ?", org.ID, user.ID).First(&member).Error; err != nil {
				t.Fatalf("membership missing: %v", err)
			}
			if member.Role != "member" {
				t.Errorf("JITRole %q produced role %q, want member", asserted, member.Role)
			}
		})
	}
}

// "admin" is the one elevated role a provider may hand out — an org admin can
// already appoint admins directly, so nothing is escalated by the shortcut.
func TestJITRoleAllowsAdmin(t *testing.T) {
	db := identityDB(t)
	org := mustOrg(t, db, "Acme")
	user := mustUser(t, db, "bob@acme.com")
	id := oidcID("https://idp.acme.com", "sub-bob", "bob@acme.com")
	if err := bindIdentity(db, user.ID, id); err != nil {
		t.Fatalf("bind: %v", err)
	}
	id.OrgID, id.AllowJIT, id.JITRole = org.ID, true, "Admin"

	if _, _, err := resolveIdentity(db, id); err != nil {
		t.Fatalf("resolveIdentity: %v", err)
	}
	var member models.OrgMember
	if err := db.Where("org_id = ? AND user_id = ?", org.ID, user.ID).First(&member).Error; err != nil {
		t.Fatalf("membership missing: %v", err)
	}
	if member.Role != "admin" {
		t.Errorf("role = %q, want admin", member.Role)
	}
}
