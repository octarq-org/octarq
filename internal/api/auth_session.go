package api

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type LogoutAllInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *LogoutAllInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LogoutAllOutputBody struct {
	OK bool `json:"ok"`
}

type LogoutAllOutput struct {
	Body LogoutAllOutputBody
}

// logoutAll deletes every session row for the caller and clears the cookie.
// POST /api/auth/logout-all
func (h *Handler) logoutAll(ctx context.Context, input *LogoutAllInput) (*LogoutAllOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	uid := h.auth.UserID(r)
	oid := h.orgID(r)
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Find(&sessions)
	ctxCtx := r.Context()
	for _, s := range sessions {
		_ = h.auth.Cache().Delete(ctxCtx, "session:"+s.Token)
	}
	h.db.Where("user_id = ?", uid).Delete(&models.Session{})
	h.auth.Clear(r, w)
	h.auditAs(r, oid, uid, "user.logout_all", "session", 0, map[string]any{"sessionsRevoked": len(sessions)})
	out := &LogoutAllOutput{}
	out.Body.OK = true
	return out, nil
}

type ListSessionsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListSessionsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SessionRow struct {
	ID         uint      `json:"id"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"userAgent"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	CreatedAt  time.Time `json:"createdAt"`
	IsCurrent  bool      `json:"isCurrent"`
	Location   string    `json:"location,omitempty"`
}

type ListSessionsOutput struct {
	Body []SessionRow
}

// GET /api/auth/sessions — list sessions for the current user, newest first.
// The session matching the caller's cookie is flagged isCurrent: true.
func (h *Handler) listSessions(ctx context.Context, input *ListSessionsInput) (*ListSessionsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	uid := h.auth.UserID(r)
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Order("last_seen_at DESC").Limit(20).Find(&sessions)

	currID := h.auth.SessionID(r)
	out := make([]SessionRow, len(sessions))
	for i, s := range sessions {
		ipClean := strings.Trim(s.IP, "[]")
		maskedIP := maskIPServer(ipClean)

		var location string
		if ipClean == "::1" || ipClean == "127.0.0.1" {
			location = "Localhost"
		} else if h.geo != nil {
			country, _, city := h.geo.Locate(ipClean)
			location = locationFromGeo(country, city)
		}
		out[i] = SessionRow{
			ID:         s.ID,
			IP:         maskedIP,
			UserAgent:  s.UserAgent,
			LastSeenAt: s.LastSeenAt,
			CreatedAt:  s.CreatedAt,
			IsCurrent:  s.ID == currID,
			Location:   location,
		}
	}
	return &ListSessionsOutput{Body: out}, nil
}

// locationFromGeo joins the non-empty parts of a geo lookup, city first.
// Both empty yields "".
func locationFromGeo(country, city string) string {
	parts := make([]string, 0, 2)
	if city != "" {
		parts = append(parts, city)
	}
	if country != "" {
		parts = append(parts, country)
	}
	return strings.Join(parts, ", ")
}

// maskIPServer redacts the last octet/group of an IP (GDPR-style).
// Localhost addresses are left as-is since they carry no PII.
func maskIPServer(ip string) string {
	if ip == "::1" || ip == "127.0.0.1" {
		return ip
	}
	// IPv4
	if parts := strings.Split(ip, "."); len(parts) == 4 {
		return parts[0] + "." + parts[1] + "." + parts[2] + ".*"
	}
	// IPv6
	if idx := strings.LastIndex(ip, ":"); idx >= 0 {
		return ip[:idx] + ":*"
	}
	return ip
}

type RevokeSessionInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *RevokeSessionInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type RevokeSessionOutputBody struct {
	OK   bool `json:"ok"`
	Self bool `json:"self"`
}

type RevokeSessionOutput struct {
	Body RevokeSessionOutputBody
}

// DELETE /api/auth/sessions/{id} — revoke a specific session row.
// With stateful cookies, just deleting the row is sufficient: the next
// request from that device will find no matching session and get a 401.
// If the caller revokes their OWN session, the cookie is also cleared.
func (h *Handler) revokeSession(ctx context.Context, input *RevokeSessionInput) (*RevokeSessionOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	uid := h.auth.UserID(r)
	oid := h.orgID(r)
	var sess models.Session
	if err := h.db.Where("id = ? AND user_id = ?", input.ID, uid).First(&sess).Error; err != nil {
		return nil, huma.Error404NotFound("session not found")
	}
	h.db.Delete(&sess)
	_ = h.auth.Cache().Delete(r.Context(), "session:"+sess.Token)
	h.auditAs(r, oid, uid, "user.session_revoke", "session", sess.ID, nil)

	// If the caller just revoked their own session, clear the cookie too.
	isSelf := h.auth.SessionID(r) == sess.ID
	if isSelf {
		h.auth.Clear(r, w)
	}
	out := &RevokeSessionOutput{}
	out.Body.OK = true
	out.Body.Self = isSelf
	return out, nil
}

type LogoutInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *LogoutInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LogoutOutputBody struct {
	OK bool `json:"ok"`
}

type LogoutOutput struct {
	Body LogoutOutputBody
}

func (h *Handler) logout(ctx context.Context, input *LogoutInput) (*LogoutOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	uid := h.auth.UserID(r)
	oid := h.orgID(r)
	sid := h.auth.SessionID(r)
	h.auth.Clear(r, w)
	h.auditAs(r, oid, uid, "user.logout", "session", sid, nil)
	out := &LogoutOutput{}
	out.Body.OK = true
	return out, nil
}

type MeInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *MeInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type MeOutputBody struct {
	Email         string `json:"email"`
	Username      string `json:"username,omitempty"`
	OrgID         uint   `json:"orgId"`
	Role          string `json:"role"`
	EmailVerified bool   `json:"emailVerified"`
	// IsInstanceAdmin is the instance identity of the caller, reported where
	// the session is: the tenant settings response still carries a copy (the
	// frontend reads it there today); this field is the new, correct source.
	IsInstanceAdmin bool `json:"isInstanceAdmin"`
}

type MeOutput struct {
	Body MeOutputBody
}

func (h *Handler) me(ctx context.Context, input *MeInput) (*MeOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("user not found")
	}
	out := &MeOutput{}
	out.Body.Email = user.Email
	out.Body.Username = user.Email
	out.Body.OrgID = h.orgID(r)
	// The role this credential can actually use — a member-capped token should
	// not report its holder as an owner to whatever is reading /me.
	out.Body.Role = string(h.effectiveRole(r))
	out.Body.EmailVerified = user.EmailVerified
	out.Body.IsInstanceAdmin = user.IsInstanceAdmin
	return out, nil
}

type ChangeEmailInputBody struct {
	NewEmail        string `json:"newEmail" doc:"The replacement email address"`
	CurrentPassword string `json:"currentPassword,omitempty" doc:"Current password for verification"`
}

type ChangeEmailInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body ChangeEmailInputBody
}

func (i *ChangeEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ChangeEmailOutputBody struct {
	OK               bool   `json:"ok"`
	Email            string `json:"email"`
	VerificationSent bool   `json:"verificationSent"`
}

type ChangeEmailOutput struct {
	Body ChangeEmailOutputBody
}

// changeEmail updates the authenticated user's email address.
// PUT /api/auth/email
func (h *Handler) changeEmail(ctx context.Context, input *ChangeEmailInput) (*ChangeEmailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	if !h.loginLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("too many attempts")
	}

	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	newEmail := strings.ToLower(strings.TrimSpace(input.Body.NewEmail))
	if addr, err := mail.ParseAddress(newEmail); err != nil || addr.Address != newEmail || !strings.Contains(newEmail, "@") {
		return nil, huma.Error400BadRequest("a valid email address is required")
	}

	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if strings.EqualFold(user.Email, newEmail) {
		out := &ChangeEmailOutput{}
		out.Body.OK = true
		out.Body.Email = user.Email
		return out, nil
	}

	if user.PasswordHash == "" {
		if user.IsInstanceAdmin {
			if input.Body.CurrentPassword == "" || !h.auth.Check(user.Email, input.Body.CurrentPassword) {
				h.loginLimiter.recordFailure(ip)
				return nil, huma.Error400BadRequest("current password is incorrect")
			}
		} else {
			return nil, huma.Error400BadRequest("this account is managed by an external identity provider; please update your email with your provider")
		}
	} else if input.Body.CurrentPassword == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Body.CurrentPassword)) != nil {
		h.loginLimiter.recordFailure(ip)
		return nil, huma.Error400BadRequest("current password is incorrect")
	}

	var existing models.User
	if h.db.Where("LOWER(email) = ? AND id <> ?", newEmail, user.ID).First(&existing).Error == nil {
		return nil, huma.NewError(http.StatusConflict, "an account with this email already exists")
	}

	oldEmail := user.Email
	verificationSent := false

	updates := map[string]any{
		"email":          newEmail,
		"email_verified": false,
	}

	if h.requireEmailVerification() {
		rawToken, tokenHash, err := generateSecureToken()
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to generate verification token")
		}
		expiry := time.Now().Add(24 * time.Hour)
		updates["verify_token_hash"] = tokenHash
		updates["verify_token_expiry"] = expiry

		verifyURL := fmt.Sprintf("%s/api/auth/verify-email?token=%s", h.origin(r), rawToken)
		h.sendVerificationEmail(newEmail, verifyURL)
		verificationSent = true
	} else {
		updates["verify_token_hash"] = ""
		updates["verify_token_expiry"] = nil
	}

	if err := h.db.Model(&user).Updates(updates).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to update email")
	}

	h.audit(r, "user.update_email", "user", user.ID, map[string]any{
		"oldEmail": oldEmail,
		"newEmail": newEmail,
	})

	// Revoke every session except the one making this request.
	currentID := h.auth.SessionID(r)
	var sessions []models.Session
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Find(&sessions)
	reqCtx := r.Context()
	for _, s := range sessions {
		_ = h.auth.Cache().Delete(reqCtx, "session:"+s.Token)
	}
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Delete(&models.Session{})

	h.loginLimiter.reset(ip)

	out := &ChangeEmailOutput{}
	out.Body.OK = true
	out.Body.Email = newEmail
	out.Body.VerificationSent = verificationSent
	return out, nil
}

type AcceptInviteInputBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type AcceptInviteInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body AcceptInviteInputBody
}

func (i *AcceptInviteInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AcceptInviteOutputBody struct {
	OK bool `json:"ok"`
}

type AcceptInviteOutput struct {
	Body AcceptInviteOutputBody
}

// POST /api/auth/invite/accept
func (h *Handler) acceptInvite(ctx context.Context, input *AcceptInviteInput) (*AcceptInviteOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	ip := reporterIP(r)
	// Same budget as the password-reset endpoints: this is an unauthenticated
	// "completion" endpoint, so every request counts (there is no "failed"
	// attempt to budget on), and sharing the budget keeps a single limiter.
	if !h.recoveryLimiter.allow(ip) {
		return nil, huma.Error429TooManyRequests("too many attempts")
	}
	h.recoveryLimiter.recordFailure(ip)

	token := strings.TrimSpace(input.Body.Token)
	if token == "" {
		return nil, huma.Error400BadRequest("token is required")
	}
	password := input.Body.Password
	if len(password) < 8 {
		return nil, huma.Error400BadRequest("password must be at least 8 characters")
	}

	var user models.User
	if err := h.db.Where("invite_token_hash = ?", hashToken(token)).First(&user).Error; err != nil {
		return nil, huma.Error400BadRequest("invalid token")
	}

	if user.InviteExpiresAt == nil || user.InviteExpiresAt.Before(time.Now()) {
		return nil, huma.Error400BadRequest("invite token has expired")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}

	user.PasswordHash = string(hash)
	user.InviteTokenHash = ""
	user.InviteExpiresAt = nil
	// Redeeming a valid, unexpired invite token proves ownership of the account's
	// address: the token is delivered only to that mailbox (see sendInviteEmail),
	// so nothing more than holding it is needed to mark the email verified. This
	// keeps invited teammates from bouncing off the login gate on instances that
	// require verification — the very users the admin just brought in.
	user.EmailVerified = true

	if err := h.db.Save(&user).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save user settings")
	}

	h.audit(r, "user.activate", "user", user.ID, map[string]any{"email": user.Email})
	// The invite is now redeemed: the account both joined its workspace(s) and
	// set its password in this one step.
	for _, oid := range h.memberOrgIDs(user.ID) {
		eventbus.Publish(oid, "member.join", map[string]any{"userId": user.ID, "email": user.Email})
		eventbus.Publish(oid, "auth.password_changed", map[string]any{"userId": user.ID, "email": user.Email})
	}

	out := &AcceptInviteOutput{}
	out.Body.OK = true
	return out, nil
}
