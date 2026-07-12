package api

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type LoginInput struct {
	Body struct {
		Username string `json:"username" doc:"The user's email address" example:"admin@example.com"`
		Password string `json:"password" doc:"The user's password" example:"securepassword"`
	}
	Ctx huma.Context `hidden:"true"`
}

func (i *LoginInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LoginOutput struct {
	Body struct {
		OK                bool   `json:"ok,omitempty"`
		Username          string `json:"username"`
		TwoFactorRequired bool   `json:"twoFactorRequired,omitempty"`
	}
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

	uid, orgID, ok := h.authenticate(input.Body.Username, input.Body.Password)
	if !ok {
		h.loginLimiter.recordFailure(ip)
		return nil, huma.Error401Unauthorized("invalid credentials")
	}

	var user models.User
	if h.db.First(&user, uid).Error == nil && user.TOTPEnabled {
		out := &LoginOutput{}
		out.Body.TwoFactorRequired = true
		out.Body.Username = input.Body.Username
		return out, nil
	}

	h.loginLimiter.reset(ip)
	h.auth.SetSessionFromRequest(r, w, uid, orgID)

	out := &LoginOutput{}
	out.Body.OK = true
	out.Body.Username = input.Body.Username
	return out, nil
}

// verify2FA completes a login that requires a second factor. The client re-sends
// username+password (re-verified here, so the challenge can't be forged) along
// with a TOTP code or a one-time recovery code. On success the session is set.
// POST /api/auth/2fa/verify  {username, password, code}
type Verify2FAInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
}

func (i *Verify2FAInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type Verify2FAOutput struct {
	Body struct {
		OK       bool   `json:"ok"`
		Username string `json:"username"`
	}
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
	uid, orgID, ok := h.authenticate(input.Body.Username, input.Body.Password)
	if !ok {
		h.loginLimiter.recordFailure(ip)
		return nil, huma.Error401Unauthorized("invalid credentials")
	}

	var user models.User
	if h.db.First(&user, uid).Error != nil {
		return nil, huma.Error401Unauthorized("invalid credentials")
	}
	if user.TOTPEnabled {
		if !h.verifyTOTPOrRecovery(&user, strings.TrimSpace(input.Body.Code)) {
			h.loginLimiter.recordFailure(ip)
			return nil, huma.Error401Unauthorized("invalid 2FA code")
		}
	}
	h.loginLimiter.reset(ip)
	h.auth.SetSessionFromRequest(r, w, uid, orgID)
	out := &Verify2FAOutput{}
	out.Body.OK = true
	out.Body.Username = input.Body.Username
	return out, nil
}

type LogoutAllInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *LogoutAllInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LogoutAllOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
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

// bootstrapUserID finds or creates the user for the admin login.
func (h *Handler) bootstrapUserID(username string, orgID uint) uint {
	var user models.User
	if err := h.db.Where("email = ?", username).First(&user).Error; err != nil {
		user = models.User{Email: username, PasswordHash: ""}
		h.db.Create(&user)
		// Link to the bootstrap org as owner.
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

type RevokeSessionOutput struct {
	Body struct {
		OK   bool `json:"ok"`
		Self bool `json:"self"`
	}
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
// doesn't exist yet. It looks up by slug (derived from AdminUser) rather than
// taking db.First(), so an OAuth user logging in before the admin password
// login cannot accidentally become the admin's org.
func (h *Handler) bootstrapOrgID() uint {
	slug := safeSlug(h.cfg.AdminUser)
	if slug == "" {
		slug = "default"
	}
	var org models.Org
	if h.db.Where("slug = ?", slug).First(&org).Error == nil {
		return org.ID
	}
	org = models.Org{Name: h.cfg.AdminUser, Slug: slug, InboundToken: uuid.NewString()}
	h.db.Create(&org)
	return org.ID
}

// safeSlug lowercases s and replaces any non-alphanumeric character with "-".
func safeSlug(s string) string {
	b := []byte(s)
	for i, c := range b {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = '-'
		}
	}
	return string(b)
}

type LogoutInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *LogoutInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LogoutOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
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

type MeOutput struct {
	Body struct {
		Username string `json:"username"`
		OrgID    uint   `json:"orgId"`
	}
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
	out.Body.Username = user.Email
	out.Body.OrgID = h.orgID(r)
	return out, nil
}

type AcceptInviteInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
}

func (i *AcceptInviteInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AcceptInviteOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
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

	out := &AcceptInviteOutput{}
	out.Body.OK = true
	return out, nil
}

type AuthConfigInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *AuthConfigInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type AuthConfigOutput struct {
	Body struct {
		GoogleEnabled       bool   `json:"googleEnabled"`
		GithubEnabled       bool   `json:"githubEnabled"`
		RegistrationEnabled bool   `json:"registrationEnabled"`
		AppName             string `json:"appName"`
	}
}

// GET /api/auth/config (public) returns whether Google and GitHub logins are enabled.
func (h *Handler) authConfig(ctx context.Context, input *AuthConfigInput) (*AuthConfigOutput, error) {
	googleEnabled := h.oauth != nil && h.getSetting(keyGoogleClientID) != "" && h.getSetting(keyGoogleClientSecret) != ""
	githubEnabled := h.oauth != nil && h.getSetting(keyGitHubClientID) != "" && h.getSetting(keyGitHubClientSecret) != ""
	
	out := &AuthConfigOutput{}
	out.Body.GoogleEnabled = googleEnabled
	out.Body.GithubEnabled = githubEnabled
	out.Body.RegistrationEnabled = h.registrationEnabled()
	out.Body.AppName = h.AppName()
	return out, nil
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
