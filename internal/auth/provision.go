package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// ErrRegistrationDisabled is returned when an unknown email tries to sign in
// through an external identity source (OAuth, SSO) on an instance that has
// public registration turned off. Existing users still resolve; only creating a
// brand-new account is refused.
var ErrRegistrationDisabled = errors.New("registration disabled")

// UpsertUserByEmail finds or creates the User + a default Org + OrgMember for a
// verified email address and returns their IDs. It is the single provisioning
// path shared by every "we trust this email" login (built-in OAuth and the Pro
// SSO plugin).
//
// Creating a brand-new user is gated on allowRegistration so an external
// identity source can't be a side door into an invite-only instance; a user
// that already exists always resolves regardless of the flag. JIT-provisioned
// users are ordinary org owners of their own personal org — never instance
// admins (that privilege is reserved for the config-backed admin credential).
func UpsertUserByEmail(db *gorm.DB, email string, allowRegistration bool) (userID, orgID uint, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return 0, 0, errors.New("empty email")
	}

	var user models.User
	e := db.Where("email = ?", email).First(&user).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		if !allowRegistration {
			return 0, 0, ErrRegistrationDisabled
		}
		user = models.User{Email: email, PasswordHash: ""}
		if err := db.Create(&user).Error; err != nil {
			return 0, 0, err
		}
	} else if e != nil {
		return 0, 0, e
	}

	// Resolve the org this user belongs to (first one wins); create a personal
	// org for a brand-new user.
	var member models.OrgMember
	me := db.Where("user_id = ?", user.ID).First(&member).Error
	if errors.Is(me, gorm.ErrRecordNotFound) {
		org := models.Org{Name: email, Slug: slugify(email), InboundToken: uuid.NewString()}
		if err := db.Create(&org).Error; err != nil {
			return 0, 0, err
		}
		member = models.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "owner"}
		if err := db.Create(&member).Error; err != nil {
			return 0, 0, err
		}
	} else if me != nil {
		return 0, 0, me
	}

	return user.ID, member.OrgID, nil
}

// registrationAllowed reports whether new accounts may be provisioned, reading
// the "allow_registration" setting directly (default on when the row is
// absent). It keeps the auth package independent of the api package.
func registrationAllowed(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	var s models.Setting
	if err := db.Where("key = ?", "allow_registration").First(&s).Error; err != nil {
		return true // no row → default on
	}
	return s.Value != "false"
}

// LoginByEmail completes a login for an already-verified email: it provisions
// (or finds) the user and issues the session cookie, exactly as built-in OAuth
// login does. It is the capability exposed to plugins via
// plugin.Context.LoginByEmail, so an SSO/identity plugin can finish login after
// it has verified an external identity (ID token, SAML assertion, …).
//
// It honours the instance's registration policy: an unknown email on an
// invite-only instance is refused with ErrRegistrationDisabled rather than
// silently provisioned. Callers MUST verify the email before calling — this
// method performs no authentication of its own.
func (m *Manager) LoginByEmail(w http.ResponseWriter, r *http.Request, email string) (userID uint, err error) {
	if m.db == nil {
		return 0, errors.New("auth: no database configured")
	}
	uid, orgID, err := UpsertUserByEmail(m.db, email, registrationAllowed(m.db))
	if err != nil {
		return 0, err
	}
	m.SetSessionFromRequest(r, w, uid, orgID)
	return uid, nil
}
