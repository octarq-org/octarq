package auth

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

var (
	providersMu sync.RWMutex
	loadedCreds = make(map[string]string) // key: provider name, value: client_id:client_secret
)

// InitGothStore sets the gorilla session store goth uses internally to hold the
// short-lived OAuth state (the CSRF nonce for the round-trip). Call once at
// startup with the same secret key used for octarq sessions. secure mirrors the
// session cookie's Secure flag so the state cookie isn't sent in cleartext over
// plain HTTP in production; SameSite=Lax and a 10-minute lifetime bound it to
// the in-flight login rather than lingering for weeks (gorilla's default).
func InitGothStore(secretKey string, secure bool) {
	store := sessions.NewCookieStore([]byte(secretKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	gothic.Store = store
}

// OAuthHandler handles OAuth begin and callback for a given provider.
// It reads provider credentials from the Settings table on each request
// so they can be changed from the dashboard without restarting.
type OAuthHandler struct {
	db           *gorm.DB
	callbackBase string // e.g. "https://app.example.com"
	auth         *Manager
	cipher       *crypto.Cipher
}

func NewOAuthHandler(db *gorm.DB, callbackBase string, auth *Manager, cipher *crypto.Cipher) *OAuthHandler {
	return &OAuthHandler{db: db, callbackBase: callbackBase, auth: auth, cipher: cipher}
}

// loadProvider registers the named provider with credentials from Settings.
// Returns false if the provider is not configured.
func (h *OAuthHandler) loadProvider(provider string) bool {
	cid := h.setting("oauth." + provider + ".client_id")
	csec := h.decryptedSetting("oauth." + provider + ".client_secret")
	if cid == "" || csec == "" {
		return false
	}

	credsKey := cid + ":" + csec

	providersMu.RLock()
	current := loadedCreds[provider]
	providersMu.RUnlock()

	if current == credsKey {
		return true
	}

	providersMu.Lock()
	defer providersMu.Unlock()

	if loadedCreds[provider] == credsKey {
		return true
	}

	cb := h.callbackBase + "/auth/callback/" + provider
	switch provider {
	case "google":
		goth.UseProviders(google.New(cid, csec, cb, "email", "profile"))
	case "github":
		goth.UseProviders(github.New(cid, csec, cb, "user:email"))
	default:
		return false
	}
	loadedCreds[provider] = credsKey
	return true
}

func (h *OAuthHandler) setting(key string) string {
	var s models.Setting
	if err := h.db.Where("key = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return s.Value
}

func (h *OAuthHandler) decryptedSetting(key string) string {
	enc := h.setting(key)
	if enc == "" {
		return ""
	}
	b, err := h.cipher.Decrypt(enc)
	if err != nil {
		return ""
	}
	return string(b)
}

// Begin starts the OAuth flow. Route: GET /auth/begin/{provider}
func (h *OAuthHandler) Begin(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !h.loadProvider(provider) {
		http.Error(w, provider+" OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	// gothic reads the provider name from the request context or query param.
	r = gothic.GetContextWithProvider(r, provider)
	gothic.BeginAuthHandler(w, r)
}

// Callback handles the OAuth redirect back from the provider.
// Route: GET /auth/callback/{provider}
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !h.loadProvider(provider) {
		http.Error(w, provider+" OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	r = gothic.GetContextWithProvider(r, provider)

	gothUser, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		log.Printf("oauth callback error (%s): %v", provider, err)
		http.Error(w, "OAuth failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(gothUser.Email))
	if email == "" {
		http.Error(w, "OAuth provider did not return an email address", http.StatusBadRequest)
		return
	}

	// Upsert User. When public registration is disabled (invite-only instance),
	// OAuth must not be a side door: existing users may still sign in, but an
	// unknown email is refused instead of silently provisioning a new account.
	user, org, err := h.upsertUser(email, gothUser.AvatarURL, provider)
	if errors.Is(err, ErrRegistrationDisabled) {
		http.Error(w, "This instance is invite-only; ask an admin to add your account first.", http.StatusForbidden)
		return
	}
	if err != nil {
		log.Printf("oauth upsert error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.auth.SetSession(w, user.ID, org.ID)
	http.Redirect(w, r, "/admin/", http.StatusFound)
}

// registrationEnabled reports whether new accounts may be provisioned. Mirrors
// api.Handler.registrationEnabled (default on unless the setting is "false") but
// reads the DB directly so the auth package stays decoupled from api.
func (h *OAuthHandler) registrationEnabled() bool {
	return registrationAllowed(h.db)
}

// upsertUser finds or creates the User + default Org + OrgMember for an
// OAuth-verified email. It delegates to the shared UpsertUserByEmail so built-in
// OAuth and the Pro SSO plugin provision users identically, then loads the rows
// the caller needs. Creating a brand-new user is gated on registrationEnabled so
// OAuth can't bypass an invite-only instance.
func (h *OAuthHandler) upsertUser(email, avatarURL, provider string) (*models.User, *models.Org, error) {
	uid, orgID, err := UpsertUserByEmail(h.db, email, h.registrationEnabled())
	if err != nil {
		return nil, nil, err
	}
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, nil, err
	}
	var org models.Org
	if err := h.db.First(&org, orgID).Error; err != nil {
		return nil, nil, err
	}
	return &user, &org, nil
}

// slugify turns an email into a URL-safe slug, e.g. "foo@bar.com" → "foo-bar-com".
func slugify(email string) string {
	r := strings.NewReplacer("@", "-", ".", "-", "_", "-", "+", "-")
	return r.Replace(strings.ToLower(email))
}
