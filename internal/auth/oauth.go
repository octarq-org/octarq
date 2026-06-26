package auth

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/models"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"
	"gorm.io/gorm"
)

// InitGothStore sets the gorilla session store goth uses internally.
// Call once at startup with the same secret key used for led sessions.
func InitGothStore(secretKey string) {
	gothic.Store = sessions.NewCookieStore([]byte(secretKey))
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
	cb := h.callbackBase + "/auth/callback/" + provider
	switch provider {
	case "google":
		goth.UseProviders(google.New(cid, csec, cb, "email", "profile"))
	case "github":
		goth.UseProviders(github.New(cid, csec, cb, "user:email"))
	default:
		return false
	}
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

	// Upsert User.
	user, org, err := h.upsertUser(email, gothUser.AvatarURL, provider)
	if err != nil {
		log.Printf("oauth upsert error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.auth.SetSession(w, user.ID, org.ID)
	http.Redirect(w, r, "/admin/", http.StatusFound)
}

// upsertUser finds or creates the User + default Org + OrgMember.
func (h *OAuthHandler) upsertUser(email, avatarURL, provider string) (*models.User, *models.Org, error) {
	var user models.User
	err := h.db.Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = models.User{Email: email, PasswordHash: ""}
		if err := h.db.Create(&user).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}

	// Find the org this user belongs to (first one wins).
	var member models.OrgMember
	if err := h.db.Where("user_id = ?", user.ID).First(&member).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		// No org yet — create a personal org for this user.
		slug := slugify(email)
		org := models.Org{Name: email, Slug: slug}
		if err := h.db.Create(&org).Error; err != nil {
			return nil, nil, err
		}
		member = models.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "owner"}
		if err := h.db.Create(&member).Error; err != nil {
			return nil, nil, err
		}
	}

	var org models.Org
	if err := h.db.First(&org, member.OrgID).Error; err != nil {
		return nil, nil, err
	}
	return &user, &org, nil
}

// slugify turns an email into a URL-safe slug, e.g. "foo@bar.com" → "foo-bar-com".
func slugify(email string) string {
	r := strings.NewReplacer("@", "-", ".", "-", "_", "-", "+", "-")
	return r.Replace(strings.ToLower(email))
}
