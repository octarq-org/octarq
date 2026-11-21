package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

var ErrAccountLinkRequired = errors.New("account link required")

var ErrIdentityBoundElsewhere = errors.New("identity already bound to another account")

func (m *Manager) LoginByIdentity(w http.ResponseWriter, r *http.Request, id plugin.ExternalIdentity) (uint, error) {
	if m.db == nil {
		return 0, errors.New("auth: no database configured")
	}
	uid, orgID, err := resolveIdentity(m.db, id)
	if err != nil {
		return 0, err
	}
	if m.userHasTOTP(uid) {
		m.auditSSOTOTPBypass(r, orgID, uid, "identity", id.Provider, id.Issuer)
	}
	m.SetSessionFromRequest(r, w, uid, orgID)
	return uid, nil
}

func (m *Manager) userHasTOTP(uid uint) bool {
	if m.db == nil {
		return false
	}
	var user models.User
	if m.db.First(&user, uid).Error != nil {
		return false
	}
	return user.TOTPEnabled
}

func (m *Manager) auditSSOTOTPBypass(r *http.Request, orgID, uid uint, method, provider, issuer string) {
	meta := map[string]any{"method": method}
	if provider != "" {
		meta["provider"] = provider
	}
	if issuer != "" {
		meta["issuer"] = issuer
	}
	metaJSON, _ := json.Marshal(meta)
	m.db.Create(&models.AuditLog{
		OrgID:      orgID,
		ActorID:    uid,
		Action:     "auth.sso_login_bypassed_totp",
		TargetType: "user",
		TargetID:   uid,
		Meta:       string(metaJSON),
		IP:         reporterIP(r),
	})
}

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
		return nil
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
	if !id.MayCreateUser || !registrationAllowed(db) {
		return 0, 0, ErrRegistrationDisabled
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		user = models.User{Email: id.Email, PasswordHash: "", EmailVerified: true}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := bindIdentity(tx, user.ID, id); err != nil {
			return err
		}
		userID, orgID, err = joinOrg(tx, user.ID, id)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	return userID, orgID, nil
}

func joinOrg(db *gorm.DB, userID uint, id plugin.ExternalIdentity) (uint, uint, error) {
	if id.OrgID == 0 {
		orgID, err := firstOrPersonalOrg(db, userID)
		return userID, orgID, err
	}

	var member models.OrgMember
	e := db.Where("org_id = ? AND user_id = ?", id.OrgID, userID).First(&member).Error
	if e == nil {
		if _, err := EnsurePersonalOrg(db, userID); err != nil {
			return 0, 0, err
		}
		return userID, id.OrgID, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return 0, 0, e
	}
	if !id.AllowJIT {
		return 0, 0, ErrRegistrationDisabled
	}
	if err := db.Create(&models.OrgMember{OrgID: id.OrgID, UserID: userID, Role: jitRole(id.JITRole)}).Error; err != nil {
		return 0, 0, err
	}
	if _, err := EnsurePersonalOrg(db, userID); err != nil {
		return 0, 0, err
	}
	return userID, id.OrgID, nil
}

func jitRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(authz.RoleAdmin):
		return string(authz.RoleAdmin)
	default:
		return string(authz.RoleMember)
	}
}

func firstOrPersonalOrg(db *gorm.DB, userID uint) (uint, error) {
	var member models.OrgMember
	e := db.Where("user_id = ?", userID).Order("org_id").First(&member).Error
	if e == nil {
		if _, err := EnsurePersonalOrg(db, userID); err != nil {
			return 0, err
		}
		return member.OrgID, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return 0, e
	}
	return EnsurePersonalOrg(db, userID)
}
