package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
)

// recoveryCodeCount is how many one-time recovery codes are minted at enrollment.
const recoveryCodeCount = 8

type TwoFAStatusInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *TwoFAStatusInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type TwoFAStatusOutputBody struct {
	Enabled bool `json:"enabled"`
}

type TwoFAStatusOutput struct {
	Body TwoFAStatusOutputBody
}

// twoFAStatus reports whether 2FA is enabled for the caller.
// GET /api/auth/2fa/status
func (h *Handler) twoFAStatus(ctx context.Context, input *TwoFAStatusInput) (*TwoFAStatusOutput, error) {
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
	out := &TwoFAStatusOutput{}
	out.Body.Enabled = user.TOTPEnabled
	return out, nil
}

type Setup2FAInputBody struct {
	// Password re-authenticates the caller before their second factor is
	// touched. See requirePasswordStepUp.
	Password string `json:"password,omitempty"`
}

type Setup2FAInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body Setup2FAInputBody
}

func (i *Setup2FAInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type Setup2FAOutput struct {
	Body map[string]any
}

// setup2FA generates a fresh (pending, not-yet-enabled) TOTP secret, stores it
// encrypted, and returns the otpauth:// URI + base32 secret so the client can
// render a QR code.
// POST /api/auth/2fa/setup
func (h *Handler) setup2FA(ctx context.Context, input *Setup2FAInput) (*Setup2FAOutput, error) {
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
	if err := h.requirePasswordStepUp(r, &user, input.Body.Password); err != nil {
		return nil, err
	}

	// The issuer is the label the authenticator app shows beside the account.
	// It must NOT come from the request host: the same user enrolling from the
	// dashboard host and later verifying from a custom domain would otherwise
	// see two different names for one account. The instance product name is
	// stable, operator-controlled, and already the brand shown in the UI.
	// (Changing it renames existing entries in users' authenticators but never
	// breaks verification — the issuer is not an input to the TOTP algorithm.)
	issuer := h.AppName()
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: user.Email,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate secret")
	}
	enc, err := h.cipher.Encrypt([]byte(key.Secret()))
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to encrypt secret")
	}
	// Store the pending secret but keep 2FA disabled until the user proves they
	// can produce a code (enable step).
	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_secret":  enc,
		"totp_enabled": false,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to store secret")
	}
	// Render the QR server-side as a data URI. The otpauth URL contains the TOTP
	// secret, so it must never be sent to a third-party QR service.
	resp := map[string]any{
		"secret":     key.Secret(),
		"otpauthUrl": key.URL(),
	}
	if png, err := qrcode.Encode(key.URL(), qrcode.Medium, 256); err == nil {
		resp["qrDataUri"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	return &Setup2FAOutput{Body: resp}, nil
}

type Enable2FAInputBody struct {
	Code string `json:"code"`
}

type Enable2FAInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body Enable2FAInputBody
}

func (i *Enable2FAInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type Enable2FAOutput struct {
	Body map[string]any
}

// enable2FA verifies a code against the pending secret and, on success, turns
// 2FA on and returns freshly minted one-time recovery codes (shown once).
// POST /api/auth/2fa/enable  {code}
func (h *Handler) enable2FA(ctx context.Context, input *Enable2FAInput) (*Enable2FAOutput, error) {
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

	if user.TOTPSecret == "" {
		return nil, huma.Error400BadRequest("2FA setup not started")
	}
	secret, err := h.cipher.Decrypt(user.TOTPSecret)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to read secret")
	}
	if !totp.Validate(strings.TrimSpace(input.Body.Code), string(secret)) {
		return nil, huma.Error400BadRequest("invalid code")
	}

	plainCodes, hashed, err := generateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate recovery codes")
	}
	hashedJSON, _ := json.Marshal(hashed)
	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_enabled":   true,
		"recovery_codes": string(hashedJSON),
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to enable 2FA")
	}
	return &Enable2FAOutput{
		Body: map[string]any{
			"ok":            true,
			"recoveryCodes": plainCodes,
		},
	}, nil
}

type Disable2FAInputBody struct {
	Code     string `json:"code,omitempty"`
	Password string `json:"password,omitempty"`
}

type Disable2FAInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body Disable2FAInputBody
}

func (i *Disable2FAInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type Disable2FAOutputBody struct {
	OK bool `json:"ok"`
}

type Disable2FAOutput struct {
	Body Disable2FAOutputBody
}

// disable2FA turns 2FA off after re-verifying the caller twice over: their
// password, and a current TOTP or recovery code.
//
// Requiring both password and TOTP/recovery code ensures 2FA cannot be removed by password alone or session alone.
// POST /api/auth/2fa/disable  {code, password}
func (h *Handler) disable2FA(ctx context.Context, input *Disable2FAInput) (*Disable2FAOutput, error) {
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

	if !user.TOTPEnabled {
		out := &Disable2FAOutput{}
		out.Body.OK = true
		return out, nil
	}

	if err := h.requirePasswordStepUp(r, &user, input.Body.Password); err != nil {
		return nil, err
	}
	if !h.verifyTOTPOrRecovery(&user, strings.TrimSpace(input.Body.Code)) {
		return nil, huma.Error401Unauthorized("verification failed")
	}

	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_enabled":   false,
		"totp_secret":    "",
		"recovery_codes": "",
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to disable 2FA")
	}
	out := &Disable2FAOutput{}
	out.Body.OK = true
	return out, nil
}

// requirePasswordStepUp re-authenticates the caller before a change to their
// own second factor. A live session is not enough authority to enrol or remove
// one: the session is precisely what an attacker has when 2FA is the only thing
// left standing between them and the account.
//
// An account with no local password — one managed by an external identity
// provider — has nothing to re-verify against, so the step-up is skipped rather
// than made impossible. Their credential lives at the IdP; octarq cannot check
// it and must not pretend to.
func (h *Handler) requirePasswordStepUp(r *http.Request, user *models.User, password string) error {
	if !h.hasLocalPassword(user) {
		return nil
	}
	if password == "" {
		return huma.Error401Unauthorized("password confirmation required")
	}
	if !h.verifyUserPassword(user, password) {
		// Same bookkeeping a failed login gets: this endpoint is otherwise a
		// password oracle that no rate limit is watching.
		h.loginLimiter.recordFailure(reporterIP(r))
		return huma.Error401Unauthorized("password confirmation failed")
	}
	return nil
}

// hasLocalPassword reports whether octarq holds a credential it can check for
// this account: a stored bcrypt hash, or the config-admin bootstrap user, whose
// password lives in the instance config rather than the row.
func (h *Handler) hasLocalPassword(user *models.User) bool {
	return user.PasswordHash != "" || h.auth.IsConfigAdmin(user.Email)
}

// verifyUserPassword checks a plaintext password against the user's own bcrypt
// hash. For the config-admin bootstrap user (which carries no stored hash), it
// falls back to the instance admin credential. This ensures a regular user
// re-authenticates with THEIR password — not the operator's — for sensitive
// actions like disabling 2FA.
func (h *Handler) verifyUserPassword(user *models.User, password string) bool {
	if user.PasswordHash != "" {
		return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
	}
	return h.auth.Check(user.Email, password)
}

// verifyTOTPOrRecovery validates code against the user's TOTP secret, or against
// their unused recovery codes (consuming a matched one). It persists recovery
// code consumption. Returns true on any successful match.
func (h *Handler) verifyTOTPOrRecovery(user *models.User, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	if user.TOTPSecret != "" {
		if user.LastTOTPCode != "" && user.LastTOTPCode == code {
			return false // Replay attack prevention
		}
		if secret, err := h.cipher.Decrypt(user.TOTPSecret); err == nil {
			if totp.Validate(code, string(secret)) {
				h.db.Model(user).Update("last_totp_code", code)
				user.LastTOTPCode = code
				return true
			}
		}
	}
	// Recovery-code path: match against the bcrypt-hashed codes and consume.
	if user.RecoveryCodes == "" {
		return false
	}
	var hashes []string
	if err := json.Unmarshal([]byte(user.RecoveryCodes), &hashes); err != nil {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(code, "-", ""))
	normalizedBytes := []byte(normalized)
	matchedIndex := -1

	for i, hh := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(hh), normalizedBytes) == nil {
			matchedIndex = i
			break
		}
	}

	if matchedIndex != -1 {
		// Consume this one-time code.
		remaining := append(append([]string{}, hashes[:matchedIndex]...), hashes[matchedIndex+1:]...)
		b, _ := json.Marshal(remaining)
		h.db.Model(user).Update("recovery_codes", string(b))
		user.RecoveryCodes = string(b)
		return true
	}
	return false
}

// generateRecoveryCodes returns n human-readable recovery codes and their
// bcrypt hashes (hashing the normalized, dash-stripped, lowercase form).
func generateRecoveryCodes(n int) (plain []string, hashed []string, err error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	for i := 0; i < n; i++ {
		buf := make([]byte, 10)
		if _, err = rand.Read(buf); err != nil {
			return nil, nil, err
		}
		raw := make([]byte, 10)
		for j := range buf {
			raw[j] = alphabet[int(buf[j])%len(alphabet)]
		}
		// Display as "abcde-fghij"; the stored/verified form strips the dash.
		display := fmt.Sprintf("%s-%s", raw[:5], raw[5:])
		hash, herr := bcrypt.GenerateFromPassword(raw, bcrypt.DefaultCost)
		if herr != nil {
			return nil, nil, herr
		}
		plain = append(plain, display)
		hashed = append(hashed, string(hash))
	}
	return plain, hashed, nil
}
