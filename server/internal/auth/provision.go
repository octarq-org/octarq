package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenancy"
	"gorm.io/gorm"
)

var ErrRegistrationDisabled = errors.New("registration disabled")

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
		user = models.User{Email: email, PasswordHash: "", EmailVerified: true}
		if err := db.Create(&user).Error; err != nil {
			return 0, 0, err
		}
	} else if e != nil {
		return 0, 0, e
	} else if !user.EmailVerified {
		db.Model(&user).Update("email_verified", true)
	}

	var member models.OrgMember
	me := db.Where("user_id = ?", user.ID).First(&member).Error
	if errors.Is(me, gorm.ErrRecordNotFound) {
		slug, err := models.AllocateOrgSlug(db)
		if err != nil {
			return 0, 0, err
		}
		org := models.Org{Name: email, Slug: slug, Kind: models.OrgKindPersonal, InboundToken: uuid.NewString()}
		if err := db.Create(&org).Error; err != nil {
			return 0, 0, err
		}
		if _, _, err := tenancy.Provision(db, org.ID, org.Slug); err != nil {
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

func registrationAllowed(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	var s models.Setting
	if err := db.Where("key = ?", "allow_registration").First(&s).Error; err != nil {
		return true
	}
	return s.Value != "false"
}

func (m *Manager) LoginByEmail(w http.ResponseWriter, r *http.Request, email string) (userID uint, err error) {
	if m.db == nil {
		return 0, errors.New("auth: no database configured")
	}
	uid, orgID, err := UpsertUserByEmail(m.db, email, registrationAllowed(m.db))
	if err != nil {
		return 0, err
	}
	if m.userHasTOTP(uid) {
		m.auditSSOTOTPBypass(r, orgID, uid, "email", "", "")
	}
	m.SetSessionFromRequest(r, w, uid, orgID)
	return uid, nil
}
