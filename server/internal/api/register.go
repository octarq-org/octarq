package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenancy"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type RegisterInputBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgName  string `json:"orgName,omitempty"`
}

type RegisterInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body RegisterInputBody
}

func (i *RegisterInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type RegisterOutputBody struct {
	OK       bool   `json:"ok"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
}

type RegisterOutput struct {
	Body RegisterOutputBody
}

func (h *Handler) register(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if !h.registrationEnabled() {
		return nil, huma.Error403Forbidden("User registration is disabled; contact an instance administrator")
	}
	ip := reporterIP(r)
	if !h.registerLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("Too many requests; wait a few minutes before trying again")
	}
	h.registerLimiter.recordFailure(ip)

	if !h.mailReady() {
		return nil, huma.NewError(http.StatusServiceUnavailable, "This instance cannot send email yet; ask the administrator to configure an SMTP sender or use an invitation")
	}
	email := strings.ToLower(strings.TrimSpace(input.Body.Email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || !strings.Contains(email, "@") {
		return nil, huma.Error400BadRequest("Valid email address required; enter an address in the form user@example.com")
	}
	if len(input.Body.Password) < 8 {
		return nil, huma.Error400BadRequest("Password must be at least 8 characters; choose a longer password")
	}
	orgName := strings.TrimSpace(input.Body.OrgName)
	if orgName != "" && len(orgName) > 255 {
		return nil, huma.Error400BadRequest("Workspace name must be 255 characters or fewer; choose a shorter name")
	}

	var existing models.User
	if h.db.Where("LOWER(email) = ?", email).First(&existing).Error == nil {
		h.sendAccountExistsEmail(email, h.origin(r)+"/admin/login")
		return registerAccepted(email), nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Body.Password), auth.BcryptCost)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not process password; retry the request")
	}

	rawToken, tokenHash, err := generateSecureToken()
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not generate verification token; retry the request")
	}
	expiry := time.Now().Add(24 * time.Hour)

	user := models.User{
		Email:             email,
		PasswordHash:      string(hash),
		EmailVerified:     false,
		VerifyTokenHash:   tokenHash,
		VerifyTokenExpiry: &expiry,
	}
	if err := h.db.Create(&user).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not create account; retry the request")
	}

	verifyURL := fmt.Sprintf("%s/api/auth/verify-email?token=%s", h.origin(r), rawToken)
	h.sendVerificationEmail(email, verifyURL)

	slug, err := models.AllocateOrgSlug(h.db)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not create workspace; retry the request")
	}
	name := email
	if orgName != "" {
		name = orgName
	}
	var orgID uint
	err = h.db.Transaction(func(tx *gorm.DB) error {
		org := models.Org{Name: name, Slug: slug, Kind: models.OrgKindTeam, InboundToken: uuid.NewString()}
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		orgID = org.ID
		if err := tx.Create(&models.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "owner"}).Error; err != nil {
			return err
		}
		_, _, err := tenancy.Provision(tx, org.ID, org.Slug)
		return err
	})
	if err != nil {
		if errors.Is(err, tenancy.ErrNameTaken) {
			return nil, huma.NewError(http.StatusConflict, "Workspace address could not be claimed; try a different workspace name")
		}
		return nil, huma.Error500InternalServerError("Could not create workspace; retry the request")
	}

	h.auditAs(r, orgID, user.ID, "user.register", "user", user.ID, map[string]any{"email": email})

	return registerAccepted(email), nil
}

func registerAccepted(email string) *RegisterOutput {
	out := &RegisterOutput{}
	out.Body.OK = true
	out.Body.Email = email
	out.Body.Username = email
	return out
}
