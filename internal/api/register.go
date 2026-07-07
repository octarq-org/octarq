package api

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/octarq-org/led/internal/models"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// POST /api/auth/register (public) — self-serve email/password sign-up.
// Gated by the instance-level allow_registration setting (default on). On
// success it provisions a fresh personal workspace with the user as owner and
// logs them straight in.
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled() {
		writeErr(w, http.StatusForbidden, "registration is disabled")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	ip := reporterIP(r)
	if !h.loginLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || !strings.Contains(email, "@") {
		writeErr(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if len(body.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// Reject duplicates case-insensitively (also covers OAuth-provisioned users).
	var existing models.User
	if h.db.Where("LOWER(email) = ?", email).First(&existing).Error == nil {
		writeErr(w, http.StatusConflict, "an account with this email already exists")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user := models.User{Email: email, PasswordHash: string(hash)}
	if err := h.db.Create(&user).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	org := models.Org{Name: email, Slug: h.uniqueOrgSlug(email), InboundToken: uuid.NewString()}
	if err := h.db.Create(&org).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}
	if err := h.db.Create(&models.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "owner"}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}

	h.audit(r, "user.register", "user", user.ID, map[string]any{"email": email})
	h.loginLimiter.reset(ip)
	h.auth.SetSessionFromRequest(r, w, user.ID, org.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": email})
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
		slug = base + "-" + randomSlug(4)
	}
}
