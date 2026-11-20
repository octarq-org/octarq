package auth

import (
	"context"
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
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/origin"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

var (
	providersMu sync.RWMutex
	loadedCreds = make(map[string]string)
)

func InitGothStore(secretKey string) {
	store := sessions.NewCookieStore(oauthStateKey(secretKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	gothic.Store = requestSecureStore{store}
}

func oauthStateKey(secretKey string) []byte {
	return crypto.DeriveKey([]byte(secretKey), crypto.PurposeOAuthState)
}

type requestSecureStore struct{ *sessions.CookieStore }

func (s requestSecureStore) New(r *http.Request, name string) (*sessions.Session, error) {
	sess, err := s.CookieStore.New(r, name)
	s.markSecure(r, sess)
	return sess, err
}

func (s requestSecureStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	sess, err := s.CookieStore.Get(r, name)
	s.markSecure(r, sess)
	return sess, err
}

func (s requestSecureStore) Save(r *http.Request, w http.ResponseWriter, sess *sessions.Session) error {
	s.markSecure(r, sess)
	return s.CookieStore.Save(r, w, sess)
}

func (s requestSecureStore) markSecure(r *http.Request, sess *sessions.Session) {
	if sess == nil {
		return
	}
	if sess.Options == nil {
		opts := *s.Options
		sess.Options = &opts
	}
	sess.Options.Secure = origin.Secure(r, trustProxy.Load())
}

type OAuthHandler struct {
	db          *gorm.DB
	origins     *origin.Resolver
	auth        *Manager
	vault       plugin.VaultService
	vaultLookup func() plugin.VaultService
}

func NewOAuthHandler(db *gorm.DB, auth *Manager, vs plugin.VaultService) *OAuthHandler {
	return &OAuthHandler{db: db, origins: origin.NewResolver(db), auth: auth, vault: vs}
}

func (h *OAuthHandler) SetVaultService(vs plugin.VaultService) {
	h.vault = vs
}

func (h *OAuthHandler) SetVaultLookup(fn func() plugin.VaultService) {
	h.vaultLookup = fn
}

func (h *OAuthHandler) resolveVault() plugin.VaultService {
	if h.vault != nil {
		return h.vault
	}
	if h.vaultLookup != nil {
		return h.vaultLookup()
	}
	return nil
}

func (h *OAuthHandler) prepare(r *http.Request, provider string) (string, bool) {
	base := h.origins.Absolute(0, r, origin.Secure(r, trustProxy.Load()))
	if base == "" {
		return "", false
	}
	name := provider + "@" + base
	if !h.loadProvider(r.Context(), provider, name, base) {
		return "", false
	}
	return name, true
}

func (h *OAuthHandler) loadProvider(ctx context.Context, provider, name, base string) bool {
	cid := h.setting("oauth." + provider + ".client_id")
	csec := h.instanceSecret(ctx, "instance.oauth."+provider)
	if cid == "" || csec == "" {
		return false
	}

	credsKey := cid + ":" + csec

	providersMu.RLock()
	current := loadedCreds[name]
	providersMu.RUnlock()

	if current == credsKey {
		return true
	}

	providersMu.Lock()
	defer providersMu.Unlock()

	if loadedCreds[name] == credsKey {
		return true
	}

	if len(loadedCreds) > 64 {
		goth.ClearProviders()
		loadedCreds = make(map[string]string)
	}

	cb := base + "/auth/callback/" + provider
	switch provider {
	case "google":
		p := google.New(cid, csec, cb, "email", "profile")
		p.SetName(name)
		goth.UseProviders(p)
	case "github":
		p := github.New(cid, csec, cb, "user:email")
		p.SetName(name)
		goth.UseProviders(p)
	default:
		return false
	}
	loadedCreds[name] = credsKey
	return true
}

func (h *OAuthHandler) setting(key string) string {
	var s models.Setting
	if err := h.db.Where("key = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return s.Value
}

func (h *OAuthHandler) instanceSecret(ctx context.Context, keyName string) string {
	vs := h.resolveVault()
	if vs == nil {
		return ""
	}
	sec, err := vs.ResolveInstanceSecret(ctx, keyName)
	if err != nil || sec == nil {
		return ""
	}
	if s, ok := sec.Payload["clientSecret"].(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if s, ok := sec.Payload["value"].(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return ""
}

func (h *OAuthHandler) Begin(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	name, ok := h.prepare(r, provider)
	if !ok {
		http.Error(w, provider+" OAuth is not available on this hostname", http.StatusServiceUnavailable)
		return
	}
	r = withProviderName(r, name)
	gothic.BeginAuthHandler(w, r)
}

func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	name, ok := h.prepare(r, provider)
	if !ok {
		http.Error(w, provider+" OAuth is not available on this hostname", http.StatusServiceUnavailable)
		return
	}
	r = withProviderName(r, name)

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

	if at := strings.LastIndex(email, "@"); at >= 0 && at < len(email)-1 {
		domain := email[at+1:]
		if h.db != nil && h.db.Migrator().HasTable("sso_configs") {
			var count int64
			if err := h.db.Table("sso_configs").Where("enabled = ? AND LOWER(allowed_domain) = ?", true, domain).Count(&count).Error; err == nil && count > 0 {
				log.Printf("oauth callback rejected: email domain %s requires enterprise SSO", domain)
				http.Error(w, "Sign-in for domain "+domain+" is managed by Enterprise SSO. Use your organization's SSO portal to sign in.", http.StatusForbidden)
				return
			}
		}
	}

	user, org, err := h.upsertUser(email)
	if errors.Is(err, ErrRegistrationDisabled) {
		http.Error(w, "This instance is invite-only; ask an admin to add your account first.", http.StatusForbidden)
		return
	}
	if err != nil {
		log.Printf("oauth upsert error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if user.TOTPEnabled {
		challenge, err := h.auth.NewTwoFAChallenge(user.ID)
		if err != nil {
			log.Printf("oauth 2fa challenge error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		h.auth.SetTwoFAChallengeCookie(w, r, challenge)
		http.Redirect(w, r, "/admin/?twofa=1", http.StatusFound)
		return
	}

	h.auth.SetSessionFromRequest(r, w, user.ID, org.ID)
	http.Redirect(w, r, "/admin/", http.StatusFound)
}

func withProviderName(r *http.Request, name string) *http.Request {
	r = gothic.GetContextWithProvider(r, name)
	r.SetPathValue("provider", name)
	return r
}

func (h *OAuthHandler) registrationEnabled() bool {
	return registrationAllowed(h.db)
}

func (h *OAuthHandler) upsertUser(email string) (*models.User, *models.Org, error) {
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
