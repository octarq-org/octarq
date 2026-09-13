package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
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

// loginHuma is the huma-adapted login handler
func (h *Handler) loginHuma(ctx context.Context, input *LoginInput) (*LoginOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)

	ip := reporterIP(r)
	if !h.loginLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("too many failed login attempts")
	}

	loginUser := strings.TrimSpace(input.Body.Email)

	uid, orgID, ok := h.authenticate(loginUser, input.Body.Password)
	if !ok {
		h.loginLimiter.recordFailure(ip)
		h.publishLoginFailed(r, loginUser, "invalid credentials")
		return nil, huma.Error401Unauthorized("invalid credentials")
	}

	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("invalid credentials")
	}

	if user.IsInstanceAdmin && !user.EmailVerified {
		h.db.Model(&user).Update("email_verified", true)
		user.EmailVerified = true
	}

	if h.requireEmailVerification() && !user.EmailVerified && !user.IsInstanceAdmin {
		return nil, huma.NewError(http.StatusForbidden, "email verification required")
	}

	if user.TOTPEnabled {
		out := &LoginOutput{}
		out.Body.TwoFactorRequired = true
		out.Body.Email = loginUser
		out.Body.Username = loginUser
		return out, nil
	}

	h.loginLimiter.reset(ip)
	h.auditAs(r, orgID, user.ID, "user.login", "user", user.ID, map[string]any{"email": loginUser})
	h.auth.SetSessionFromRequest(r, w, user.ID, orgID)

	out := &LoginOutput{}
	out.Body.OK = true
	out.Body.Email = loginUser
	out.Body.Username = loginUser
	return out, nil
}

// bootstrapUserID finds or creates the user for the admin login. This account —
// keyed on the configured OCTARQ_ADMIN_USER email — is the ONLY instance admin;
// it is marked with IsInstanceAdmin here so the privilege is bound to a stable
// identity rather than to org_id ordering (see isInstanceAdmin).
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
	// Link to the bootstrap org as owner. Unconditional, because bootstrapOrgID
	// now finds the admin's org *through* this membership: leave an admin user
	// with no membership and every login would mint another org.
	var member models.OrgMember
	if err := h.db.Where("user_id = ?", user.ID).Order("org_id").First(&member).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		h.db.Create(&models.OrgMember{OrgID: orgID, UserID: user.ID, Role: "owner"})
	}
	return user.ID
}

// bootstrapOrgID returns the ID of the admin's own org, creating it if it
// doesn't exist yet.
//
// It resolves through the admin's own user row and membership, never db.First()
// on orgs, so an OAuth user who signed in before the first admin password login
// cannot accidentally become the admin's org. This used to key on a slug derived
// from AdminUser; slugs are random now (models.AllocateOrgSlug), so the identity
// that survives a restart is the admin credential's email, not its slug. Existing
// instances resolve to the same org either way — the admin user row and its
// owner membership were created alongside that org.
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
	org := models.Org{Name: h.cfg.AdminUser, Slug: slug, InboundToken: uuid.NewString()}
	if err := h.db.Create(&org).Error; err != nil {
		return 0
	}
	return org.ID
}

// memberOrgIDs returns the IDs of every workspace the user belongs to, for
// fan-out of account-level events (join, password change, failed login) that
// have no single-org request context.
func (h *Handler) memberOrgIDs(userID uint) []uint {
	var orgIDs []uint
	h.db.Model(&models.OrgMember{}).Where("user_id = ?", userID).Pluck("org_id", &orgIDs)
	return orgIDs
}

// publishLoginFailed emits auth.login_failed to every workspace the target
// account belongs to, and records it in audit_logs for the same workspaces.
// Unknown usernames publish nowhere — there is no org to notify, and firing on
// arbitrary strings would let probes spam webhooks. The stored meta carries
// email + reason only: never the attempted password nor any token.
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

// GET /api/auth/config (public) returns whether Google and GitHub logins are
// enabled, plus the branding the login screen should wear.
//
// Branding is resolved per workspace: the session's org when the caller already
// has one, otherwise the workspace that owns the request hostname (a tenant on
// their own domain sees their own brand; the shared host falls back to the
// instance default). Host is client-controlled, so it selects PRESENTATION only
// — no data and no privilege hangs off it here.
func (h *Handler) authConfig(ctx context.Context, input *AuthConfigInput) (*AuthConfigOutput, error) {
	googleEnabled := h.oauth != nil && h.getSetting(keyGoogleClientID) != "" && h.getSetting(keyGoogleClientSecret) != ""
	githubEnabled := h.oauth != nil && h.getSetting(keyGitHubClientID) != "" && h.getSetting(keyGitHubClientSecret) != ""

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

// authenticate resolves username+password to a (userID, orgID) pair. It accepts
// two credential sources, in order:
//  1. the instance admin credential (config-backed) → the admin's bootstrap org
//  2. a database user carrying a bcrypt password hash (invited members and
//     self-serve sign-ups) → the first org they belong to
//
// It returns ok=false when neither matches. Callers arm the failed-login limiter.
func (h *Handler) authenticate(username, password string) (uid, orgID uint, ok bool) {
	if h.auth.Check(username, password) {
		orgID = h.bootstrapOrgID()
		return h.bootstrapUserID(username, orgID), orgID, true
	}

	email := strings.ToLower(strings.TrimSpace(username))
	var user models.User
	if h.db.Where("LOWER(email) = ?", email).First(&user).Error != nil {
		return 0, 0, false
	}
	if user.PasswordHash != "" {
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
			return 0, 0, false
		}
	} else if user.IsInstanceAdmin {
		if !h.auth.CheckAdminPassword(password) {
			return 0, 0, false
		}
	} else {
		return 0, 0, false
	}
	var member models.OrgMember
	if h.db.Where("user_id = ?", user.ID).Order("org_id").First(&member).Error != nil {
		orgID = h.bootstrapOrgID()
		h.db.Create(&models.OrgMember{OrgID: orgID, UserID: user.ID, Role: "owner"})
		return user.ID, orgID, true
	}
	return user.ID, member.OrgID, true
}
