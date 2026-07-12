package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

type TwoFAStatusOutput struct {
	Body struct {
		Enabled bool `json:"enabled"`
	}
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

type Setup2FAInput struct {
	Ctx huma.Context `hidden:"true"`
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

	issuer := "octarq"
	if h.cfg.AdminHost != "" {
		issuer = h.cfg.AdminHost
	}
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

type Enable2FAInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Code string `json:"code"`
	}
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

type Disable2FAInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Code     string `json:"code,omitempty"`
		Password string `json:"password,omitempty"`
	}
}

func (i *Disable2FAInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type Disable2FAOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// disable2FA turns 2FA off after re-verifying the caller with either a current
// TOTP/recovery code or their password.
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

	verified := false
	if code := strings.TrimSpace(input.Body.Code); code != "" {
		verified = h.verifyTOTPOrRecovery(&user, code)
	}
	if !verified && input.Body.Password != "" {
		verified = h.verifyUserPassword(&user, input.Body.Password)
	}
	if !verified {
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
	for i, hh := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(hh), []byte(normalized)) == nil {
			// Consume this one-time code.
			remaining := append(append([]string{}, hashes[:i]...), hashes[i+1:]...)
			b, _ := json.Marshal(remaining)
			h.db.Model(user).Update("recovery_codes", string(b))
			user.RecoveryCodes = string(b)
			return true
		}
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
