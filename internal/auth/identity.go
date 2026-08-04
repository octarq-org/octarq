package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// ErrAccountLinkRequired is returned when a verified external identity is not
// bound to any user yet, but the email it asserts already belongs to one.
//
// Refusing is the whole point. Once each tenant configures its own SSO issuer
// and client credentials, the email claim is written by whoever set up that
// org, so treating it as proof of who is signing in hands over any account
// whose address the attacker can guess. The binding has to be created from the
// authenticated side instead — see BindIdentity.
var ErrAccountLinkRequired = errors.New("account link required")

// ErrIdentityBoundElsewhere is returned by BindIdentity when the identity is
// already attached to a different user. Rebinding is not offered: it would let
// whoever controls the IdP move an existing binding onto an account they hold.
var ErrIdentityBoundElsewhere = errors.New("identity already bound to another account")

// LoginByIdentity completes a login for an externally verified identity, keyed
// on (provider, issuer, subject). It backs plugin.Context.LoginByIdentity;
// callers MUST have verified the assertion, as this performs no authentication
// of its own. plugin.ExternalIdentity documents the decision table.
//
// It exists alongside LoginByEmail because per-org SSO changes who writes the
// assertion. With one instance-wide IdP chosen by the instance admin, "this
// email is verified" was as trustworthy as the admin. With an issuer per org,
// filled in by that org, it is only as trustworthy as the least trustworthy
// tenant — so the email stops being an identifier and (issuer, subject) becomes
// one. LoginByEmail stays for built-in OAuth, whose providers are instance-wide.
func (m *Manager) LoginByIdentity(w http.ResponseWriter, r *http.Request, id plugin.ExternalIdentity) (uint, error) {
	if m.db == nil {
		return 0, errors.New("auth: no database configured")
	}
	uid, orgID, err := resolveIdentity(m.db, id)
	if err != nil {
		return 0, err
	}
	m.SetSessionFromRequest(r, w, uid, orgID)
	return uid, nil
}

// BindIdentity attaches a verified external identity to the user already
// signed in on r. It backs plugin.Context.BindIdentity and is the resolution
// for ErrAccountLinkRequired: the account's owner authenticates the way they
// already can, then adds the identity from account settings. Because the
// request must carry their session, controlling an IdP that asserts their
// address is not enough to reach the account.
func (m *Manager) BindIdentity(r *http.Request, id plugin.ExternalIdentity) error {
	if m.db == nil {
		return errors.New("auth: no database configured")
	}
	uid := m.UserID(r)
	if uid == 0 {
		return errors.New("auth: not signed in")
	}
	return bindIdentity(m.db, uid, id)
}

// normalizeIdentity trims and lowercases the parts that are case-insensitive
// and rejects an assertion missing anything the key needs.
func normalizeIdentity(id plugin.ExternalIdentity) (plugin.ExternalIdentity, error) {
	id.Provider = strings.ToLower(strings.TrimSpace(id.Provider))
	id.Issuer = plugin.NormalizeIssuer(id.Issuer)
	id.Subject = strings.TrimSpace(id.Subject)
	id.Email = strings.ToLower(strings.TrimSpace(id.Email))
	if id.Provider == "" || id.Issuer == "" || id.Subject == "" {
		return id, errors.New("auth: identity needs provider, issuer and subject")
	}
	return id, nil
}

// bindIdentity writes the (provider, issuer, subject) → user binding, refusing
// to move one that already points somewhere else.
func bindIdentity(db *gorm.DB, userID uint, id plugin.ExternalIdentity) error {
	id, err := normalizeIdentity(id)
	if err != nil {
		return err
	}
	var existing models.UserIdentity
	err = db.Where("provider = ? AND issuer = ? AND subject = ?", id.Provider, id.Issuer, id.Subject).
		First(&existing).Error
	if err == nil {
		if existing.UserID != userID {
			return ErrIdentityBoundElsewhere
		}
		return nil // already bound to this user; binding twice is a no-op
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(&models.UserIdentity{
		UserID:   userID,
		Provider: id.Provider,
		Issuer:   id.Issuer,
		Subject:  id.Subject,
		Email:    id.Email,
	}).Error
}

// resolveIdentity runs the decision table and returns the user to sign in and
// the org to make active.
func resolveIdentity(db *gorm.DB, id plugin.ExternalIdentity) (userID, orgID uint, err error) {
	id, err = normalizeIdentity(id)
	if err != nil {
		return 0, 0, err
	}

	var bound models.UserIdentity
	e := db.Where("provider = ? AND issuer = ? AND subject = ?", id.Provider, id.Issuer, id.Subject).
		First(&bound).Error
	switch {
	case e == nil:
		return joinOrg(db, bound.UserID, id)
	case !errors.Is(e, gorm.ErrRecordNotFound):
		return 0, 0, e
	}

	// Unbound. Whether we may go on depends on whether the asserted address
	// already names somebody — and if it does, the answer is always no.
	if id.Email == "" {
		return 0, 0, errors.New("auth: identity has no email claim")
	}
	var user models.User
	e = db.Where("email = ?", id.Email).First(&user).Error
	if e == nil {
		return 0, 0, ErrAccountLinkRequired
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return 0, 0, e
	}
	// Minting a global identity is an instance-level decision; an org admin
	// configuring their own issuer does not get to make it. The instance's own
	// invite-only switch still wins on top of the per-IdP approval — it is the
	// same policy LoginByEmail honours, and "invite only" has to mean it.
	if !id.MayCreateUser || !registrationAllowed(db) {
		return 0, 0, ErrRegistrationDisabled
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		// JIT-provisioned users are never instance admins — that privilege is
		// bound to the configured admin credential, not to any assertion.
		user = models.User{Email: id.Email, PasswordHash: "", EmailVerified: true}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := bindIdentity(tx, user.ID, id); err != nil {
			return err
		}
		// Inside the transaction so a refused join rolls the new user back
		// rather than leaving an account nobody can reach.
		userID, orgID, err = joinOrg(tx, user.ID, id)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	return userID, orgID, nil
}

// joinOrg resolves which org the session lands in for an authenticated user:
// the one the login URL named if they belong to it (or may join it now), and
// otherwise refuses. It never touches membership of any other org — arriving
// through one org's SSO must not change standing anywhere else.
func joinOrg(db *gorm.DB, userID uint, id plugin.ExternalIdentity) (uint, uint, error) {
	if id.OrgID == 0 {
		// No org named (instance-wide SSO): fall back to the user's first org,
		// creating a personal one if they somehow have none.
		orgID, err := firstOrPersonalOrg(db, userID)
		return userID, orgID, err
	}

	var member models.OrgMember
	e := db.Where("org_id = ? AND user_id = ?", id.OrgID, userID).First(&member).Error
	if e == nil {
		return userID, id.OrgID, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return 0, 0, e
	}
	if !id.AllowJIT {
		return 0, 0, ErrRegistrationDisabled
	}
	role := strings.TrimSpace(id.JITRole)
	if role == "" {
		role = "member"
	}
	if err := db.Create(&models.OrgMember{OrgID: id.OrgID, UserID: userID, Role: role}).Error; err != nil {
		return 0, 0, err
	}
	return userID, id.OrgID, nil
}

// firstOrPersonalOrg returns the user's existing org, provisioning a personal
// one if they have none. It mirrors the tail of UpsertUserByEmail.
func firstOrPersonalOrg(db *gorm.DB, userID uint) (uint, error) {
	var member models.OrgMember
	e := db.Where("user_id = ?", userID).Order("org_id").First(&member).Error
	if e == nil {
		return member.OrgID, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return 0, e
	}
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		return 0, err
	}
	slug, err := models.AllocateOrgSlug(db)
	if err != nil {
		return 0, err
	}
	org := models.Org{Name: user.Email, Slug: slug, InboundToken: uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		return 0, err
	}
	if err := db.Create(&models.OrgMember{OrgID: org.ID, UserID: userID, Role: "owner"}).Error; err != nil {
		return 0, err
	}
	return org.ID, nil
}
