package api

import (
	"context"
	"net/http"
	"net/mail"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type RegisterInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
}

func (i *RegisterInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type RegisterOutput struct {
	Body struct {
		OK       bool   `json:"ok"`
		Username string `json:"username"`
	}
}

// POST /api/auth/register (public) — self-serve email/password sign-up.
// Gated by the instance-level allow_registration setting (default on). On
// success it provisions a fresh personal workspace with the user as owner and
// logs them straight in.
func (h *Handler) register(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if !h.registrationEnabled() {
		return nil, huma.Error403Forbidden("registration is disabled")
	}
	ip := reporterIP(r)
	if !h.loginLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("too many attempts")
	}
	email := strings.ToLower(strings.TrimSpace(input.Body.Email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || !strings.Contains(email, "@") {
		return nil, huma.Error400BadRequest("a valid email is required")
	}
	if len(input.Body.Password) < 8 {
		return nil, huma.Error400BadRequest("password must be at least 8 characters")
	}

	// Reject duplicates case-insensitively (also covers OAuth-provisioned users).
	var existing models.User
	if h.db.Where("LOWER(email) = ?", email).First(&existing).Error == nil {
		return nil, huma.NewError(http.StatusConflict, "an account with this email already exists")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}

	user := models.User{Email: email, PasswordHash: string(hash)}
	if err := h.db.Create(&user).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to create account")
	}

	org := models.Org{Name: email, Slug: h.uniqueOrgSlug(email), InboundToken: uuid.NewString()}
	if err := h.db.Create(&org).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to create workspace")
	}
	if err := h.db.Create(&models.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "owner"}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to create workspace")
	}

	h.audit(r, "user.register", "user", user.ID, map[string]any{"email": email})
	h.loginLimiter.reset(ip)
	h.auth.SetSessionFromRequest(r, w, user.ID, org.ID)
	out := &RegisterOutput{}
	out.Body.OK = true
	out.Body.Username = email
	return out, nil
}

// uniqueOrgSlug derives a URL-safe slug from an email and guarantees it neither
// collides with an existing org nor with a reserved slug, appending a short
// random suffix until it's free.
func (h *Handler) uniqueOrgSlug(email string) string {
	base := safeSlug(email)
	if base == "" {
		base = "workspace"
	}
	slug := base
	for {
		var n int64
		h.db.Model(&models.Org{}).Where("slug = ?", slug).Count(&n)
		if n == 0 && !h.isReservedSlug(slug) {
			return slug
		}
		slug = base + "-" + models.RandomSlug(4)
	}
}
