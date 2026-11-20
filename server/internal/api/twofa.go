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
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
)

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

func (h *Handler) twoFAStatus(ctx context.Context, input *TwoFAStatusInput) (*TwoFAStatusOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("User account not found; verify the member identifier and try again")
	}
	out := &TwoFAStatusOutput{}
	out.Body.Enabled = user.TOTPEnabled
	return out, nil
}

type Setup2FAInputBody struct {
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

func (h *Handler) setup2FA(ctx context.Context, input *Setup2FAInput) (*Setup2FAOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("User account not found; verify the member identifier and try again")
	}
	if err := h.requirePasswordStepUp(r, &user, input.Body.Password); err != nil {
		return nil, err
	}

	issuer := h.AppName()
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: user.Email,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not generate two-factor authentication secret; retry the request")
	}
	enc, err := h.cipher.Encrypt([]byte(key.Secret()))
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not encrypt two-factor authentication secret; retry the request")
	}
	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_secret":  enc,
		"totp_enabled": false,
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not save two-factor authentication secret; retry the request")
	}
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

func (h *Handler) enable2FA(ctx context.Context, input *Enable2FAInput) (*Enable2FAOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("User account not found; verify the member identifier and try again")
	}

	if user.TOTPSecret == "" {
		return nil, huma.Error400BadRequest("Two-factor authentication setup has not been initiated; start setup before enabling")
	}
	secret, err := h.cipher.Decrypt(user.TOTPSecret)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not load two-factor authentication secret; retry the request")
	}
	if !totp.Validate(strings.TrimSpace(input.Body.Code), string(secret)) {
		return nil, huma.Error400BadRequest("Invalid verification code; check the code and try again")
	}

	plainCodes, hashed, err := generateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not generate recovery codes; retry the request")
	}
	hashedJSON, _ := json.Marshal(hashed)
	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_enabled":   true,
		"recovery_codes": string(hashedJSON),
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not enable two-factor authentication; retry the request")
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

func (h *Handler) disable2FA(ctx context.Context, input *Disable2FAInput) (*Disable2FAOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error401Unauthorized("User account not found; verify the member identifier and try again")
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
		return nil, huma.Error401Unauthorized("Two-factor verification failed; check the code and try again")
	}

	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_enabled":   false,
		"totp_secret":    "",
		"recovery_codes": "",
	}).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not disable two-factor authentication; retry the request")
	}
	out := &Disable2FAOutput{}
	out.Body.OK = true
	return out, nil
}

func (h *Handler) requirePasswordStepUp(r *http.Request, user *models.User, password string) error {
	if !h.hasLocalPassword(user) {
		return nil
	}
	if password == "" {
		return huma.Error401Unauthorized("Current password is required; enter your password to confirm")
	}
	if !h.verifyUserPassword(user, password) {
		h.loginLimiter.recordFailure(reporterIP(r))
		return huma.Error401Unauthorized("Incorrect password; check your password and try again")
	}
	return nil
}

func (h *Handler) hasLocalPassword(user *models.User) bool {
	return user.PasswordHash != "" || h.auth.IsConfigAdmin(user.Email)
}

func (h *Handler) verifyUserPassword(user *models.User, password string) bool {
	if user.PasswordHash != "" {
		return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
	}
	return h.auth.Check(user.Email, password)
}

func (h *Handler) verifyTOTPOrRecovery(user *models.User, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	if user.TOTPSecret != "" {
		if user.LastTOTPCode != "" && user.LastTOTPCode == code {
			return false
		}
		if secret, err := h.cipher.Decrypt(user.TOTPSecret); err == nil {
			if totp.Validate(code, string(secret)) {
				h.db.Model(user).Update("last_totp_code", code)
				user.LastTOTPCode = code
				return true
			}
		}
	}
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
		remaining := append(append([]string{}, hashes[:matchedIndex]...), hashes[matchedIndex+1:]...)
		b, _ := json.Marshal(remaining)
		h.db.Model(user).Update("recovery_codes", string(b))
		user.RecoveryCodes = string(b)
		return true
	}
	return false
}

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
		display := fmt.Sprintf("%s-%s", raw[:5], raw[5:])
		hash, herr := bcrypt.GenerateFromPassword(raw, auth.BcryptCost)
		if herr != nil {
			return nil, nil, herr
		}
		plain = append(plain, display)
		hashed = append(hashed, string(hash))
	}
	return plain, hashed, nil
}
