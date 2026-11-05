package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/models"
)

type Verify2FAInputBody struct {
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"` // required for password logins; absent for OAuth challenge logins
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

// verify2FA completes a login that requires a second factor. The client re-sends
// email+password (re-verified here, so the challenge can't be forged) along
// with a TOTP code or a one-time recovery code. On success the session is set.
//
// OAuth logins have no password to re-send; the callback instead mints a
// short-lived signed challenge (auth.Manager.NewTwoFAChallenge), stores it in
// an HttpOnly cookie, and redirects to the same 2FA page, which submits only
// the code. Both branches share the TOTP/recovery verification and session
// issuance below — the second-factor step is one mechanism, whichever proof
// started it.
// POST /api/auth/2fa/verify  {email, password, code} | {code + challenge cookie}
func (h *Handler) verify2FA(ctx context.Context, input *Verify2FAInput) (*Verify2FAOutput, error) {
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

	var (
		uid, orgID uint
		user       models.User
		meta       map[string]any
		challenge  bool // login rides on an OAuth 2FA challenge cookie
	)
	if token := h.auth.TwoFAChallengeFromRequest(r); token != "" {
		challenge = true
		uid = h.auth.VerifyTwoFAChallenge(token)
		if uid == 0 {
			// Forged, expired or malformed: the challenge is dead and no retry
			// can revive it, so clear the cookie instead of letting the browser
			// resubmit it. (A wrong TOTP code does NOT get here and must not
			// clear the cookie — the challenge is still good, and clearing
			// would turn a typo into a full OAuth restart.)
			h.auth.ClearTwoFAChallengeCookie(w, r)
			h.recordLoginFailure(ip, account)
			return nil, huma.Error401Unauthorized("Invalid two-factor authentication challenge; sign in again to receive a new challenge")
		}
		if err := h.db.First(&user, uid).Error; err != nil {
			h.recordLoginFailure(ip, account)
			return nil, huma.Error401Unauthorized("Invalid two-factor authentication challenge; sign in again to receive a new challenge")
		}
		// A challenge exists only for logins pending a second factor; an
		// account that disabled TOTP since the challenge was minted must start
		// its login over rather than ride the stale challenge to a session.
		if !user.TOTPEnabled {
			h.recordLoginFailure(ip, account)
			return nil, huma.Error401Unauthorized("Invalid two-factor authentication challenge; sign in again to receive a new challenge")
		}
		var member models.OrgMember
		if err := h.db.Where("user_id = ?", user.ID).First(&member).Error; err != nil {
			h.recordLoginFailure(ip, account)
			return nil, huma.Error401Unauthorized("Invalid two-factor authentication challenge; sign in again to receive a new challenge")
		}
		orgID = member.OrgID
		loginUser = user.Email
		// A challenge sign-in may carry no email in the body, so the account
		// budget becomes chargeable only once the challenge names the user.
		if account == "" {
			account = accountLoginKey(user.Email)
			if !h.loginAccountLimiter.allow(account) {
				return nil, huma.Error429TooManyRequests("Too many failed login attempts; wait a few minutes before trying again")
			}
		}
		meta = map[string]any{"email": user.Email, "method": "oauth"}
	} else {
		var err error
		uid, orgID, err = h.authenticate(loginUser, input.Body.Password)
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
		if err := h.db.First(&user, uid).Error; err != nil {
			return nil, huma.Error401Unauthorized("Invalid credentials; check your email and password and try again")
		}
		meta = map[string]any{"email": loginUser}
	}
	if user.IsInstanceAdmin && !user.EmailVerified {
		h.db.Model(&user).Update("email_verified", true)
		user.EmailVerified = true
	}
	if h.requireEmailVerification() && !user.EmailVerified && !user.IsInstanceAdmin {
		return nil, apierror.New(http.StatusForbidden, apierror.CodeEmailVerificationRequired, "Email verification required; verify your email before signing in")
	}
	if user.TOTPEnabled {
		if !h.verifyTOTPOrRecovery(&user, strings.TrimSpace(input.Body.Code)) {
			h.recordLoginFailure(ip, account)
			h.publishLoginFailed(r, loginUser, "invalid 2FA code")
			return nil, huma.Error401Unauthorized("Invalid two-factor authentication code; check the code from your authenticator app and try again")
		}
	}
	h.resetLoginFailures(ip, account)
	h.auditAs(r, orgID, user.ID, "user.login", "user", user.ID, meta)
	h.auth.SetSessionFromRequest(r, w, uid, orgID)
	if challenge {
		// The challenge is spent the moment a session exists for it: expire
		// the cookie in the same response so it cannot be replayed or linger.
		h.auth.ClearTwoFAChallengeCookie(w, r)
	}
	out := &Verify2FAOutput{}
	out.Body.OK = true
	out.Body.Email = loginUser
	out.Body.Username = loginUser
	return out, nil
}
