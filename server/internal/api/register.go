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
	// OrgName is the display name for the provisioned personal workspace.
	// Optional: when blank the registration email is used, preserving the
	// pre-naming behavior for callers that don't send it.
	OrgName string `json:"orgName,omitempty"`
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
	// VerificationRequired reports that the account exists but no session
	// was established: the instance requires a verified email first. The
	// client must branch on this field, not on whether a cookie came back.
	VerificationRequired bool `json:"verificationRequired"`
}

type RegisterOutput struct {
	Body RegisterOutputBody
}

// POST /api/auth/register (public) — self-serve email/password sign-up.
// Gated by the instance-level allow_registration setting (default on) and an
// independent per-IP budget (registerLimiter, 5/hour) that counts every
// request. On success it provisions a fresh personal workspace with the user
// as owner and logs them straight in — unless the instance requires a verified
// email, in which case no session is set and the response says so. When
// verification is required but the instance cannot send mail, registration
// fails with 503 instead of promising a verification email that can never
// arrive.
func (h *Handler) register(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, w := humago.Unwrap(input.Ctx)
	if !h.registrationEnabled() {
		return nil, huma.Error403Forbidden("User registration is disabled; contact an instance administrator")
	}
	ip := reporterIP(r)
	// Own budget, not the login one: register succeeds without ever counting
	// on the login limiter (which only Peek()s and counts failures), so the
	// shared budget was never spent and could never stop a sign-up flood.
	// Every request counts here, exactly like recoveryLimiter.
	if !h.registerLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("Too many requests; wait a few minutes before trying again")
	}
	h.registerLimiter.recordFailure(ip)

	// Fail loudly instead of returning an unfulfillable verificationRequired:
	// a verification email is only deliverable when some SMTP sender exists.
	if h.requireEmailVerification() && !h.mailReady() {
		return nil, huma.NewError(http.StatusServiceUnavailable, "This instance cannot send email yet; ask the administrator to configure an SMTP sender or disable email verification")
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

	// Reject duplicates case-insensitively (also covers OAuth-provisioned users).
	var existing models.User
	if h.db.Where("LOWER(email) = ?", email).First(&existing).Error == nil {
		return nil, huma.NewError(http.StatusConflict, "An account with this email address already exists; use a different email address")
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
	// Sent before the org/membership transaction below: at this point the user
	// belongs to no workspace, so the email goes through the instance's system
	// sender — an org-scoped sender lookup would find nothing and drop the mail.
	h.sendVerificationEmail(email, verifyURL)

	slug, err := models.AllocateOrgSlug(h.db)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not create workspace; retry the request")
	}
	name := email
	if orgName != "" {
		name = orgName
	}
	// Org + membership + the tenant subdomain claim (when a base domain is
	// configured) are one transaction: if the address cannot be provisioned the
	// workspace does not silently come into being half-set-up.
	var orgID uint
	err = h.db.Transaction(func(tx *gorm.DB) error {
		org := models.Org{Name: name, Slug: slug, InboundToken: uuid.NewString()}
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

	out := &RegisterOutput{}
	out.Body.OK = true
	out.Body.Email = email
	out.Body.Username = email

	// Same answer as the login path (auth.go) for the same state: with the
	// instance-level gate on, an unverified account gets no session. Login's
	// extra !user.IsInstanceAdmin escape hatch has no counterpart here — that
	// flag is only ever set for the bootstrap operator account (see
	// bootstrapUserID in auth.go), never for a self-serve sign-up, so the
	// freshly created user below is always a plain member.
	if h.requireEmailVerification() && !user.EmailVerified {
		out.Body.VerificationRequired = true
		return out, nil
	}

	h.auth.SetSessionFromRequest(r, w, user.ID, orgID)
	return out, nil
}
