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
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/crypto/bcrypt"
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
	h.audit(r, "user.login", "user", user.ID, map[string]any{"email": loginUser})
	h.auth.SetSessionFromRequest(r, w, user.ID, orgID)

	out := &LoginOutput{}
	out.Body.OK = true
	out.Body.Email = loginUser
	out.Body.Username = loginUser
	return out, nil
}

// verify2FA completes a login that requires a second factor. The client re-sends
// email+password (re-verified here, so the challenge can't be forged) along
// with a TOTP code or a one-time recovery code. On success the session is set.
// POST /api/auth/2fa/verify  {email, password, code}
type Verify2FAInputBody struct {
	Email    string `json:"email,omitempty"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

type Verify2FAInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body Verify2FAInputBody
}

func (i *Verify2FAInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type Verify2FAOutputBody struct {
	OK       bool   `json:"ok"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
}

type Verify2FAOutput struct {
	Body Verify2FAOutputBody
}

func (h *Handler) verify2FA(ctx context.Context, input *Verify2FAInput) (*Verify2FAOutput, error) {
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
	if h.db.First(&user, uid).Error != nil {
		return nil, huma.Error401Unauthorized("invalid credentials")
	}
	if h.requireEmailVerification() && !user.EmailVerified && !user.IsInstanceAdmin {
		return nil, huma.NewError(http.StatusForbidden, "email verification required")
	}
	if user.TOTPEnabled {
		if !h.verifyTOTPOrRecovery(&user, strings.TrimSpace(input.Body.Code)) {
			h.loginLimiter.recordFailure(ip)
			h.publishLoginFailed(r, loginUser, "invalid 2FA code")
			return nil, huma.Error401Unauthorized("invalid 2FA code")
		}
	}
	h.loginLimiter.reset(ip)
	h.auth.SetSessionFromRequest(r, w, uid, orgID)
	out := &Verify2FAOutput{}
	out.Body.OK = true
	out.Body.Email = loginUser
	out.Body.Username = loginUser
	return out, nil
}

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
	var sessions []models.Session
	h.db.Where("user_id = ?", uid).Find(&sessions)
	ctxCtx := r.Context()
	for _, s := range sessions {
		_ = h.auth.Cache().Delete(ctxCtx, "session:"+s.Token)
	}
	h.db.Where("user_id = ?", uid).Delete(&models.Session{})
	h.auth.Clear(r, w)
	out := &LogoutAllOutput{}
	out.Body.OK = true
	return out, nil
}

type ChangePasswordInputBody struct {
	CurrentPassword string `json:"currentPassword" doc:"The password the account signs in with today"`
	NewPassword     string `json:"newPassword" doc:"The replacement, at least 8 characters"`
}

type ChangePasswordInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body ChangePasswordInputBody
}

func (i *ChangePasswordInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ChangePasswordOutputBody struct {
	OK bool `json:"ok"`
}

type ChangePasswordOutput struct {
	Body ChangePasswordOutputBody
}

// changePassword replaces the caller's own password, given the current one.
// POST /api/auth/password
//
// This is the authenticated counterpart to the emailed reset flow: the same
// 8-character floor, and the same session cleanup, except the caller's own
// session survives so changing a password doesn't sign you out of the page you
// changed it on. Every OTHER session dies — that is the point of the endpoint
// as much as the new hash is, since "someone else is logged in as me" is the
// reason people change a password in a hurry.
//
// Deliberately not bumping SessionEpoch, which resetPassword does: the field is
// written in exactly one place and read in none, so it invalidates nothing.
// Deleting the session rows and their cache entries is what actually revokes
// access, and doing only the thing that works beats doing both and leaving the
// next reader unsure which one matters.
func (h *Handler) changePassword(ctx context.Context, input *ChangePasswordInput) (*ChangePasswordOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	newPassword := input.Body.NewPassword
	if len(newPassword) < 8 {
		return nil, huma.Error400BadRequest("password must be at least 8 characters")
	}

	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// An empty hash means this account has no password of its own to replace:
	// it signs in through an OAuth provider, or it is the env-configured
	// bootstrap admin whose credentials live in OCTARQ_ADMIN_PASSWORD. Writing a
	// hash for either would be worse than refusing — the bootstrap admin would
	// still authenticate with the env password (h.auth.Check runs first), so the
	// change would appear to work and change nothing.
	if user.PasswordHash == "" {
		return nil, huma.Error400BadRequest("this account signs in without a stored password; use the password reset flow or your identity provider")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Body.CurrentPassword)) != nil {
		return nil, huma.Error400BadRequest("current password is incorrect")
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}
	// Clear any outstanding reset token along with the password. Changing your
	// password from inside the app is the natural response to "I got a reset
	// email I didn't ask for", and it has to actually end that link: the token
	// stays valid for its full hour otherwise, and redeeming it overwrites the
	// password just chosen and deletes every session — so the defensive action
	// hands the account over instead of protecting it.
	if err := h.db.Model(&user).Updates(map[string]any{
		"password_hash":      string(pwHash),
		"reset_token_hash":   "",
		"reset_token_expiry": nil,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to update password")
	}

	// Revoke every session except the one making this request.
	currentID := h.auth.SessionID(r)
	var sessions []models.Session
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Find(&sessions)
	reqCtx := r.Context()
	for _, s := range sessions {
		_ = h.auth.Cache().Delete(reqCtx, "session:"+s.Token)
	}
	h.db.Where("user_id = ? AND id <> ?", uid, currentID).Delete(&models.Session{})

	out := &ChangePasswordOutput{}
	out.Body.OK = true
	return out, nil
}

// bootstrapUserID finds or creates the user for the admin login. This account —
// keyed on the configured OCTARQ_ADMIN_USER email — is the ONLY instance admin;
// it is marked with IsInstanceAdmin here so the privilege is bound to a stable
// identity rather than to org_id ordering (see isInstanceAdmin).
func (h *Handler) bootstrapUserID(username string, orgID uint) uint {
	var user models.User
	if err := h.db.Where("email = ?", username).First(&user).Error; err != nil {
		user = models.User{Email: username, PasswordHash: "", IsInstanceAdmin: true, EmailVerified: true}
		h.db.Create(&user)
	} else if !user.IsInstanceAdmin {
		// Backfill for accounts created before this column existed.
		h.db.Model(&user).Update("is_instance_admin", true)
	}
	// Link to the bootstrap org as owner. Unconditional, because bootstrapOrgID
	// now finds the admin's org *through* this membership: leave an admin user
	// with no membership and every login would mint another org.
	var member models.OrgMember
	if h.db.Where("user_id = ?", user.ID).Order("id").First(&member).Error != nil {
		h.db.Create(&models.OrgMember{OrgID: orgID, UserID: user.ID, Role: "owner"})
	}
	return user.ID
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
			if city != "" && country != "" {
				location = city + ", " + country
			} else if country != "" {
				location = country
			} else if city != "" {
				location = city
			}
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
	var sess models.Session
	if err := h.db.Where("id = ? AND user_id = ?", input.ID, uid).First(&sess).Error; err != nil {
		return nil, huma.Error404NotFound("session not found")
	}
	h.db.Delete(&sess)
	_ = h.auth.Cache().Delete(r.Context(), "session:"+sess.Token)

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
	if h.db.Where("email = ?", h.cfg.AdminUser).First(&user).Error == nil {
		var member models.OrgMember
		if h.db.Where("user_id = ?", user.ID).Order("id").First(&member).Error == nil {
			return member.OrgID
		}
	}
	slug, err := models.AllocateOrgSlug(h.db)
	if err != nil {
		return 0
	}
	org := models.Org{Name: h.cfg.AdminUser, Slug: slug, InboundToken: uuid.NewString()}
	h.db.Create(&org)
	return org.ID
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
	h.auth.Clear(r, w)
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
		return nil, huma.Error400BadRequest("this account is managed by an external identity provider; please update your email with your provider")
	}

	if input.Body.CurrentPassword == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Body.CurrentPassword)) != nil {
		h.loginLimiter.recordFailure(ip)
		return nil, huma.Error400BadRequest("current password is incorrect")
	}

	// Check if another account already uses this email
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
		h.sendVerificationEmail(user.ID, newEmail, verifyURL)
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
	token := strings.TrimSpace(input.Body.Token)
	if token == "" {
		return nil, huma.Error400BadRequest("token is required")
	}
	password := input.Body.Password
	if len(password) < 8 {
		return nil, huma.Error400BadRequest("password must be at least 8 characters")
	}

	var user models.User
	if err := h.db.Where("invite_token = ?", token).First(&user).Error; err != nil {
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
	user.InviteToken = ""
	user.InviteExpiresAt = nil

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

// memberOrgIDs returns the IDs of every workspace the user belongs to, for
// fan-out of account-level events (join, password change, failed login) that
// have no single-org request context.
func (h *Handler) memberOrgIDs(userID uint) []uint {
	var orgIDs []uint
	h.db.Model(&models.OrgMember{}).Where("user_id = ?", userID).Pluck("org_id", &orgIDs)
	return orgIDs
}

// publishLoginFailed emits auth.login_failed to every workspace the target
// account belongs to. Unknown usernames publish nowhere — there is no org to
// notify, and firing on arbitrary strings would let probes spam webhooks.
func (h *Handler) publishLoginFailed(r *http.Request, username, reason string) {
	var user models.User
	if h.db.Where("email = ?", strings.TrimSpace(username)).First(&user).Error != nil {
		return
	}
	for _, oid := range h.memberOrgIDs(user.ID) {
		eventbus.Publish(oid, "auth.login_failed", map[string]any{"email": user.Email, "reason": reason, "ip": reporterIP(r)})
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
	if user.PasswordHash == "" {
		return 0, 0, false
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return 0, 0, false
	}
	var member models.OrgMember
	if h.db.Where("user_id = ?", user.ID).First(&member).Error != nil {
		return 0, 0, false
	}
	return user.ID, member.OrgID, true
}
