package auth

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/origin"
	"gorm.io/gorm"
)

var (
	providersMu sync.RWMutex
	loadedCreds = make(map[string]string) // key: provider name, value: client_id:client_secret
)

// InitGothStore sets the gorilla session store goth uses internally to hold the
// short-lived OAuth state (the CSRF nonce for the round-trip). Call once at
// startup with the same secret key used for octarq sessions. SameSite=Lax and a
// 10-minute lifetime bound the state to the in-flight login rather than letting
// it linger for weeks (gorilla's default).
func InitGothStore(secretKey string) {
	store := sessions.NewCookieStore([]byte(secretKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	gothic.Store = requestSecureStore{store}
}

// requestSecureStore marks the OAuth state cookie Secure exactly when the
// request carrying the login arrived over TLS, mirroring what the session
// cookie does (Manager.setCookie). A fixed flag could only ever be wrong for
// one of the two schemes an instance answers on: set over plain HTTP the
// browser drops the state cookie and every login fails with "could not find a
// matching session"; unset over HTTPS the nonce travels in cleartext.
//
// The flag has to be stamped on the SESSION, not applied in Save: a session
// carries a pointer to the store that created it and saves through that, so
// gothic's `session.Save(req, res)` reaches CookieStore.Save directly and any
// logic here would be skipped. New and Get are the only hooks that see both the
// request and the session.
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
	// CookieStore hands each session its own copy of Options, so this
	// per-request write cannot leak into the next request's session.
	if sess.Options == nil {
		opts := *s.Options
		sess.Options = &opts
	}
	sess.Options.Secure = origin.Secure(r, trustProxy)
}

// OAuthHandler handles OAuth begin and callback for a given provider.
// It reads provider credentials from the Settings table on each request
// so they can be changed from the dashboard without restarting.
type OAuthHandler struct {
	db      *gorm.DB
	origins *origin.Resolver
	auth    *Manager
	cipher  *crypto.Cipher
}

func NewOAuthHandler(db *gorm.DB, auth *Manager, cipher *crypto.Cipher) *OAuthHandler {
	return &OAuthHandler{db: db, origins: origin.NewResolver(db), auth: auth, cipher: cipher}
}

// prepare resolves the origin this login round-trip must run on and registers
// the provider for it, returning the goth provider name to use.
//
// OPERATOR-VISIBLE BEHAVIOUR: redirect_uri is now derived from the request
// instead of from one configured base URL, so it is
// "<origin>/auth/callback/<provider>" for whichever registered hostname the
// user started the login on. Providers reject any redirect_uri that is not
// registered with them verbatim, so every hostname an operator offers login on
// must be added at the provider. A host that is not registered in the domains
// table resolves to no origin at all and login is refused there rather than
// sent out with a guessed callback.
//
// Each (provider, origin) pair is registered under its own goth name. Goth's
// provider registry is a process-wide map keyed by name, so re-registering
// "google" per request would let one request's callback URL be swapped out from
// under another request on a different hostname.
func (h *OAuthHandler) prepare(r *http.Request, provider string) (string, bool) {
	base := h.origins.Absolute(0, r, origin.Secure(r, trustProxy))
	if base == "" {
		return "", false
	}
	name := provider + "@" + base
	if !h.loadProvider(provider, name, base) {
		return "", false
	}
	return name, true
}

// loadProvider registers the named provider, under goth name, with credentials
// from Settings and a callback URL rooted at base. Returns false if the
// provider is not configured.
func (h *OAuthHandler) loadProvider(provider, name, base string) bool {
	cid := h.setting("oauth." + provider + ".client_id")
	csec := h.decryptedSetting("oauth." + provider + ".client_secret")
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

	// On an instance with no registered domains the origin falls back to the
	// request host (see origin), so the set of names is caller-
	// influenced. Bound it: a sprayer would otherwise grow goth's process-wide
	// registry without limit. Wholesale reset beats LRU bookkeeping here — the
	// cost of a reset is one extra provider registration per live hostname.
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
	name, ok := h.prepare(r, provider)
	if !ok {
		http.Error(w, provider+" OAuth is not available on this hostname", http.StatusServiceUnavailable)
		return
	}
	r = withProviderName(r, name)
	gothic.BeginAuthHandler(w, r)
}

// Callback handles the OAuth redirect back from the provider.
// Route: GET /auth/callback/{provider}
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	// The callback must resolve to the same origin the login began on: that is
	// what the provider echoed back and what keyed the state in the session.
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

	// Upsert User. When public registration is disabled (invite-only instance),
	// OAuth must not be a side door: existing users may still sign in, but an
	// unknown email is refused instead of silently provisioning a new account.
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

	// A TOTP-enabled account must prove its second factor before any session
	// exists — the OAuth round-trip is not a substitute for it (R3-twofa).
	// The browser is sent to the SAME second-factor page password login uses,
	// carrying a short-lived signed challenge in place of the password the
	// OAuth account does not have; /api/auth/2fa/verify completes it via
	// challengeToken and mints the session through the shared tail of
	// verify2FA. No parallel pending state is introduced — one challenge
	// endpoint, one session-issuance path.
	if user.TOTPEnabled {
		challenge, err := h.auth.NewTwoFAChallenge(user.ID)
		if err != nil {
			log.Printf("oauth 2fa challenge error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/?twofa="+url.QueryEscape(challenge), http.StatusFound)
		return
	}

	h.auth.SetSessionFromRequest(r, w, user.ID, org.ID)
	http.Redirect(w, r, "/admin/", http.StatusFound)
}

// withProviderName tells gothic which registered goth provider to use.
//
// gothic resolves the name from several places in a fixed order and consults
// the path value BEFORE the context (see gothic.GetProviderName), so the
// {provider} segment of the route — a bare "google" — would win over anything
// put in the context. Both are set here, to the same per-origin name.
func withProviderName(r *http.Request, name string) *http.Request {
	r = gothic.GetContextWithProvider(r, name)
	r.SetPathValue("provider", name)
	return r
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
