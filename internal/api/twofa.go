package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
)

// recoveryCodeCount is how many one-time recovery codes are minted at enrollment.
const recoveryCodeCount = 8

// currentUser loads the session user's row, or writes a 401 and returns false.
func (h *Handler) currentUser(w http.ResponseWriter, r *http.Request) (models.User, bool) {
	uid := h.auth.UserID(r)
	if uid == 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return models.User{}, false
	}
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		writeErr(w, http.StatusUnauthorized, "user not found")
		return models.User{}, false
	}
	return user, true
}

// twoFAStatus reports whether 2FA is enabled for the caller.
// GET /api/auth/2fa/status
func (h *Handler) twoFAStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": user.TOTPEnabled})
}

// setup2FA generates a fresh (pending, not-yet-enabled) TOTP secret, stores it
// encrypted, and returns the otpauth:// URI + base32 secret so the client can
// render a QR code.
// POST /api/auth/2fa/setup
func (h *Handler) setup2FA(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
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
		writeErr(w, http.StatusInternalServerError, "failed to generate secret")
		return
	}
	enc, err := h.cipher.Encrypt([]byte(key.Secret()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to encrypt secret")
		return
	}
	// Store the pending secret but keep 2FA disabled until the user proves they
	// can produce a code (enable step).
	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_secret":  enc,
		"totp_enabled": false,
	}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to store secret")
		return
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
	writeJSON(w, http.StatusOK, resp)
}

// enable2FA verifies a code against the pending secret and, on success, turns
// 2FA on and returns freshly minted one-time recovery codes (shown once).
// POST /api/auth/2fa/enable  {code}
func (h *Handler) enable2FA(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if user.TOTPSecret == "" {
		writeErr(w, http.StatusBadRequest, "2FA setup not started")
		return
	}
	secret, err := h.cipher.Decrypt(user.TOTPSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read secret")
		return
	}
	if !totp.Validate(strings.TrimSpace(body.Code), string(secret)) {
		writeErr(w, http.StatusBadRequest, "invalid code")
		return
	}

	plainCodes, hashed, err := generateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate recovery codes")
		return
	}
	hashedJSON, _ := json.Marshal(hashed)
	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_enabled":   true,
		"recovery_codes": string(hashedJSON),
	}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to enable 2FA")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"recoveryCodes": plainCodes,
	})
}

// disable2FA turns 2FA off after re-verifying the caller with either a current
// TOTP/recovery code or their password.
// POST /api/auth/2fa/disable  {code, password}
func (h *Handler) disable2FA(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !user.TOTPEnabled {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	verified := false
	if code := strings.TrimSpace(body.Code); code != "" {
		verified = h.verifyTOTPOrRecovery(&user, code)
	}
	if !verified && body.Password != "" {
		verified = h.verifyUserPassword(&user, body.Password)
	}
	if !verified {
		writeErr(w, http.StatusUnauthorized, "verification failed")
		return
	}

	if err := h.db.Model(&user).Updates(map[string]any{
		"totp_enabled":   false,
		"totp_secret":    "",
		"recovery_codes": "",
	}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to disable 2FA")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
		if secret, err := h.cipher.Decrypt(user.TOTPSecret); err == nil {
			if totp.Validate(code, string(secret)) {
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
