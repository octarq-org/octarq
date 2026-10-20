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
	// No session is issued and no flag says so. A successful sign-up always
	// means the same thing — a message is on its way to the address, and the
	// next step is in it — so the response carries no field the client could
	// read as "sometimes there is a session instead". The two outcomes the
	// endpoint hides (a fresh address, an address that already has an
	// account) must be indistinguishable down to the cookies, and a branch
	// on a constant would only invite one back.
}

type RegisterOutput struct {
	Body RegisterOutputBody
}

// POST /api/auth/register (public) — self-serve email/password sign-up.
// Gated by the instance-level allow_registration setting (default on) and an
// independent per-IP budget (registerLimiter, 5/hour) that counts every
// request.
//
// The answer never says whether the address already has an account. This
// endpoint is public, so a reply that distinguishes the two turns it into an
// oracle for "who has an account on this instance" — the same reason the
// password-reset endpoint answers 200 whatever it finds. Both outcomes answer
// the same way: a message is on its way to the address, and that message says
// what to do next (verify the address, or sign in to the account that is
// already there). The owner of the mailbox is the only one who learns which
// of the two it was, and they already know.
//
// A consequence, and the reason the response carries no session: the two
// outcomes must be indistinguishable down to the cookies, so registration can
// only ever end in the mailbox. A sign-up that handed out a session at once
// would differ from a refused one by that cookie alone.
//
// The instance must be able to send mail: the answer promises a message, and
// a message that cannot be delivered is a promise nobody can keep. An
// instance without a mail sender takes sign-ups through invitations instead.
func (h *Handler) register(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
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

	// Fail loudly instead of promising a message that can never arrive.
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

	// The address may or may not already have an account. The answer must not
	// say which: this endpoint is public, and a reply that distinguishes the
	// two turns it into an oracle for "who has an account here". So the taken
	// case creates nothing and sends the owner of the address a message that
	// says what to do next; the free case creates the account and sends the
	// verification link. The reply is the same either way.
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

	return registerAccepted(email), nil
}

// registerAccepted is the one answer this endpoint gives. It is deliberately
// the same for a fresh address and for an address that already has an account:
// the client learns that a message is on its way and nothing else.
//
// The session is never set here. A session would tell the two cases apart by
// its cookie alone, and the whole point of the uniform answer is that the
// client cannot tell them apart. The mailbox hands out the session instead:
// the verification link marks the address verified, and the user signs in.
func registerAccepted(email string) *RegisterOutput {
	out := &RegisterOutput{}
	out.Body.OK = true
	out.Body.Email = email
	out.Body.Username = email
	return out
}
