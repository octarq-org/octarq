package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type LoginInputBody struct {
	Email    string `json:"email,omitempty" doc:"The user's email address" example:"admin@example.com"`
	Password string `json:"password" doc:"The user's password" example:"securepassword"`
}

type LoginInput struct {
	Body LoginInputBody
	Ctx  huma.Context `hidden:"true"`
}

func (i *LoginInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LoginOutputBody struct {
	OK                bool   `json:"ok,omitempty"`
	Email             string `json:"email"`
	Username          string `json:"username,omitempty"`
	TwoFactorRequired bool   `json:"twoFactorRequired,omitempty"`
}

type LoginOutput struct {
	Body LoginOutputBody
}

func (h *Handler) loginHuma(ctx context.Context, input *LoginInput) (*LoginOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, w := humago.Unwrap(input.Ctx)

	ip := reporterIP(r)
	loginUser := strings.TrimSpace(input.Body.Email)
	account := accountLoginKey(loginUser)
	if !h.loginAllowed(ip, account) {
		return nil, huma.Error429TooManyRequests("Too many failed login attempts; wait a few minutes before trying again")
	}

	uid, orgID, err := h.authenticate(loginUser, input.Body.Password)
	if err != nil {
		switch {
		case errors.Is(err, errNoWorkspace):
			return nil, apierror.New(http.StatusForbidden, apierror.CodeNoWorkspace, "This account has no workspace; ask an administrator for an invitation")
		case errors.Is(err, errInvalidCredentials):
			h.recordLoginFailure(ip, account)
			h.publishLoginFailed(r, loginUser, "invalid credentials")
			return nil, huma.Error401Unauthorized("Invalid credentials; check your email and password and try again")
		default:
			return nil, huma.Error500InternalServerError("Could not read the account; retry the request")
		}
	}

	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("Invalid credentials; check your email and password and try again")
	}

	if user.IsInstanceAdmin && !user.EmailVerified {
		h.db.Model(&user).Update("email_verified", true)
		user.EmailVerified = true
	}

	if h.requireEmailVerification() && !user.EmailVerified && !user.IsInstanceAdmin {
		return nil, apierror.New(http.StatusForbidden, apierror.CodeEmailVerificationRequired, "Email verification required; verify your email before signing in")
	}

	if user.TOTPEnabled {
		out := &LoginOutput{}
		out.Body.TwoFactorRequired = true
		out.Body.Email = loginUser
		out.Body.Username = loginUser
		return out, nil
	}

	h.resetLoginFailures(ip, account)
	h.auditAs(r, orgID, user.ID, "user.login", "user", user.ID, map[string]any{"email": loginUser})
	h.auth.SetSessionFromRequest(r, w, user.ID, orgID)

	out := &LoginOutput{}
	out.Body.OK = true
	out.Body.Email = loginUser
	out.Body.Username = loginUser
	return out, nil
}

func (h *Handler) bootstrapUserID(username string, orgID uint) uint {
	var user models.User
	if err := h.db.Where("email = ?", username).First(&user).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		if adminErr := h.db.Where("is_instance_admin = ?", true).First(&user).Error; errors.Is(adminErr, gorm.ErrRecordNotFound) {
			user = models.User{Email: username, PasswordHash: "", IsInstanceAdmin: true, EmailVerified: true}
			h.db.Create(&user)
		} else if adminErr != nil {
			return 0
		}
	} else if err != nil {
		return 0
	} else {
		updates := map[string]any{}
		if !user.IsInstanceAdmin {
			updates["is_instance_admin"] = true
			user.IsInstanceAdmin = true
		}
		if !user.EmailVerified {
			updates["email_verified"] = true
			user.EmailVerified = true
		}
		if len(updates) > 0 {
			h.db.Model(&user).Updates(updates)
		}
	}
	var member models.OrgMember
	if err := h.db.Where("user_id = ?", user.ID).Order("org_id").First(&member).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		var orgExists int64
		if err := h.db.Model(&models.Org{}).Where("id = ?", orgID).Count(&orgExists).Error; err != nil {
			return 0
		}
		if orgExists > 0 {
			if err := h.db.Create(&models.OrgMember{OrgID: orgID, UserID: user.ID, Role: "owner"}).Error; err != nil {
				return 0
			}
		}
	}
	if _, err := auth.EnsurePersonalOrg(h.db, user.ID); err != nil {
		return 0
	}
	return user.ID
}

func (h *Handler) bootstrapOrgID() uint {
	var user models.User
	if err := h.db.Where("email = ?", h.cfg.AdminUser).First(&user).Error; err == nil {
		var member models.OrgMember
		if err := h.db.Where("user_id = ?", user.ID).Order("org_id").First(&member).Error; err == nil {
			return member.OrgID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0
	} else if err := h.db.Where("is_instance_admin = ?", true).First(&user).Error; err == nil {
		var member models.OrgMember
		if err := h.db.Where("user_id = ?", user.ID).Order("org_id").First(&member).Error; err == nil {
			return member.OrgID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0
		}
	}
	slug, err := models.AllocateOrgSlug(h.db)
	if err != nil {
		return 0
	}
	org := models.Org{Name: h.cfg.AdminUser, Slug: slug, Kind: models.OrgKindTeam, InboundToken: uuid.NewString()}
	if err := h.db.Create(&org).Error; err != nil {
		return 0
	}
	return org.ID
}

func (h *Handler) memberOrgIDs(userID uint) []uint {
	var orgIDs []uint
	h.db.Model(&models.OrgMember{}).Where("user_id = ?", userID).Pluck("org_id", &orgIDs)
	return orgIDs
}

func (h *Handler) publishLoginFailed(r *http.Request, username, reason string) {
	var user models.User
	if h.db.Where("email = ?", strings.TrimSpace(username)).First(&user).Error != nil {
		return
	}
	for _, oid := range h.memberOrgIDs(user.ID) {
		eventbus.Publish(oid, "auth.login_failed", map[string]any{"email": user.Email, "reason": reason, "ip": reporterIP(r)})
		h.auditAs(r, oid, user.ID, "auth.login_failed", "user", user.ID, map[string]any{"email": user.Email, "reason": reason})
	}
}

type AuthConfigInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *AuthConfigInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AuthConfigOutputBody struct {
	GoogleEnabled       bool   `json:"googleEnabled"`
	GithubEnabled       bool   `json:"githubEnabled"`
	RegistrationEnabled bool   `json:"registrationEnabled"`
	AppName             string `json:"appName"`
	LogoURL             string `json:"logoUrl"`
	BrandColor          string `json:"brandColor"`
	BrandColor2         string `json:"brandColor2"`
}

type AuthConfigOutput struct {
	Body AuthConfigOutputBody
}

func (h *Handler) authConfig(ctx context.Context, input *AuthConfigInput) (*AuthConfigOutput, error) {
	googleEnabled := h.oauth != nil && h.getSetting(keyGoogleClientID) != "" && h.hasInstanceSecret(ctx, vaultKeyGoogleClientSecret, "clientSecret", "value")
	githubEnabled := h.oauth != nil && h.getSetting(keyGitHubClientID) != "" && h.hasInstanceSecret(ctx, vaultKeyGitHubClientSecret, "clientSecret", "value")

	orgID := uint(0)
	if input.Ctx != nil {
		if r, _ := humago.Unwrap(input.Ctx); r != nil {
			orgID = h.brandingOrg(r)
		}
	}

	out := &AuthConfigOutput{}
	out.Body.GoogleEnabled = googleEnabled
	out.Body.GithubEnabled = githubEnabled
	out.Body.RegistrationEnabled = h.registrationEnabled()
	out.Body.AppName = h.AppNameFor(orgID)
	out.Body.LogoURL, out.Body.BrandColor, out.Body.BrandColor2 = h.BrandFor(orgID)
	return out, nil
}

type AuthMethodsOutput struct {
	Body []auth.AuthMethod
}

func (h *Handler) getAuthMethods(ctx context.Context, input *struct{}) (*AuthMethodsOutput, error) {
	return &AuthMethodsOutput{Body: auth.List()}, nil
}

var (
	errInvalidCredentials = errors.New("auth: invalid credentials")
	errNoWorkspace        = errors.New("auth: account belongs to no workspace")
)

func (h *Handler) authenticate(username, password string) (uid, orgID uint, err error) {
	if h.auth.Check(username, password) {
		orgID = h.bootstrapOrgID()
		return h.bootstrapUserID(username, orgID), orgID, nil
	}

	email := strings.ToLower(strings.TrimSpace(username))
	var user models.User
	if h.db.Where("LOWER(email) = ?", email).First(&user).Error != nil {
		return 0, 0, errInvalidCredentials
	}
	if user.PasswordHash != "" {
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
			return 0, 0, errInvalidCredentials
		}
	} else if user.IsInstanceAdmin {
		if !h.auth.CheckAdminPassword(password) {
			return 0, 0, errInvalidCredentials
		}
	} else {
		return 0, 0, errInvalidCredentials
	}
	var member models.OrgMember
	memberErr := h.db.Where("user_id = ?", user.ID).Order("org_id").First(&member).Error
	switch {
	case memberErr == nil:
		return user.ID, member.OrgID, nil
	case errors.Is(memberErr, gorm.ErrRecordNotFound):
		return 0, 0, errNoWorkspace
	default:
		return 0, 0, memberErr
	}
}
