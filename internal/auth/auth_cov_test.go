package auth

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/sessions"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// fakeCache is a tiny in-memory cache.Cache for exercising the session cache
// hit/expiry paths that cache.New("") (a Noop) never reaches.
type fakeCache struct {
	mu sync.Mutex
	m  map[string]any
}

func newFakeCache() *fakeCache { return &fakeCache{m: map[string]any{}} }

func (f *fakeCache) Get(_ context.Context, key string, dst any) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.m[key]
	if !ok {
		return false
	}
	*dst.(*models.Session) = *v.(*models.Session)
	return true
}

func (f *fakeCache) Set(_ context.Context, key string, val any, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := *val.(*models.Session)
	f.m[key] = &s
	return nil
}

func (f *fakeCache) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, key)
	return nil
}

func (f *fakeCache) IsRedis() bool { return false }

// TestManagerTokenAndAdmin covers the small context/credential helpers: token
// identity extraction and the constant-time admin check.
func TestManagerTokenAndAdmin(t *testing.T) {
	m := testManager(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if id := m.TokenID(req); id != 0 {
		t.Errorf("TokenID without context = %d, want 0", id)
	}
	if role := TokenRoleFromContext(req.Context()); role != "" {
		t.Errorf("TokenRoleFromContext absent = %q, want empty", role)
	}

	ctx := WithTokenID(req.Context(), 42)
	ctx = WithTokenRole(ctx, "scoped")
	req2 := req.WithContext(ctx)
	if id := m.TokenID(req2); id != 42 {
		t.Errorf("TokenID = %d, want 42", id)
	}
	if role := TokenRoleFromContext(ctx); role != "scoped" {
		t.Errorf("TokenRoleFromContext = %q, want scoped", role)
	}

	// WithOrgID routes through the shared plugin package.
	if got := plugin.OrgIDFromContext(WithOrgID(req.Context(), 7)); got != 7 {
		t.Errorf("WithOrgID = %d, want 7", got)
	}

	if !m.Check("admin", "pw") {
		t.Error("Check rejected the configured admin")
	}
	if m.Check("admin", "wrong") || m.Check("nobody", "pw") {
		t.Error("Check accepted a wrong credential")
	}
	if !m.CheckAdminPassword("pw") {
		t.Error("CheckAdminPassword rejected the configured admin password")
	}
	if m.CheckAdminPassword("wrong") {
		t.Error("CheckAdminPassword accepted a wrong password")
	}
}

// TestTokenByRequestExpired validates that an expired bearer token row is not
// admitted even when its hash matches.
func TestTokenByRequestExpired(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)
	raw := "oct_expired_token_00000000000000000000000"
	expired := time.Now().Add(-time.Hour)
	tok := models.Token{
		Name: "expired", Hash: models.HashToken(raw), Prefix: raw[:8],
		OrgID: 1, ExpiresAt: &expired,
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	if got, ok := m.TokenByRequest(req); ok {
		t.Errorf("expired token admitted: %+v", got)
	}
}

// TestRevokeUserOrgTokens deletes exactly the tokens bound to one (user, org)
// and leaves a token of the same user in another org alone.
func TestRevokeUserOrgTokens(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)
	mustUser(t, db, "u@acme.com")

	seed := func(name string, owner uint) {
		raw := name + strings.Repeat("x", 26)
		db.Create(&models.Token{Name: name, Hash: models.HashToken(raw), Prefix: raw[:8], UserID: 1, OrgID: owner})
	}
	seed("in-org", 5)
	seed("other-org", 6)

	// No-op guards.
	if n := m.RevokeUserOrgTokens(0, 5); n != 0 {
		t.Errorf("RevokeUserOrgTokens(user 0) = %d, want 0", n)
	}

	if n := m.RevokeUserOrgTokens(1, 5); n != 1 {
		t.Fatalf("RevokeUserOrgTokens = %d, want 1", n)
	}
	var remaining int64
	db.Model(&models.Token{}).Where("user_id = ?", 1).Count(&remaining)
	if remaining != 1 {
		t.Errorf("remaining tokens = %d, want the other-org token only", remaining)
	}
}

// TestSetSessionFromRequestRefresh covers the same-fingerprint refresh: a
// session with a matching cookie reuses its raw token while switching the org,
// and one without a cookie rotates to a fresh token.
func TestSetSessionFromRequestRefresh(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)

	first := httptest.NewRequest(http.MethodGet, "/", nil)
	first.Header.Set("User-Agent", "ua")
	recFirst := httptest.NewRecorder()
	m.SetSessionFromRequest(first, recFirst, 1, 5)
	cookie1 := sessionCookie(t, recFirst)
	if cookie1 == "" {
		t.Fatal("no session cookie issued")
	}

	// Refreshing from the same browser with the same cookie reuses the token.
	second := httptest.NewRequest(http.MethodGet, "/", nil)
	second.Header.Set("User-Agent", "ua")
	second.AddCookie(&http.Cookie{Name: cookieName, Value: cookie1})
	recSecond := httptest.NewRecorder()
	m.SetSessionFromRequest(second, recSecond, 1, 9)
	if cookie2 := sessionCookie(t, recSecond); cookie2 != cookie1 {
		t.Errorf("matching-cookie refresh rotated the token: %q vs %q", cookie1, cookie2)
	}
	var s models.Session
	if err := db.Where("token = ?", models.HashToken(cookie1)).First(&s).Error; err != nil {
		t.Fatalf("session row gone: %v", err)
	}
	if s.OrgID != 9 {
		t.Errorf("row org after refresh = %d, want 9", s.OrgID)
	}

	// Same fingerprint but no cookie rotates to a fresh token.
	third := httptest.NewRequest(http.MethodGet, "/", nil)
	third.Header.Set("User-Agent", "ua")
	recThird := httptest.NewRecorder()
	m.SetSessionFromRequest(third, recThird, 1, 5)
	if cookie3 := sessionCookie(t, recThird); cookie3 == cookie1 || cookie3 == "" {
		t.Errorf("no-cookie refresh kept the old token (%q)", cookie3)
	}
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			return c.Value
		}
	}
	return ""
}

// TestSessionCacheHitAndExpiry proves sessionByToken prefers a live cache row
// and drops an expired cached row before falling back to the database.
func TestSessionCacheHitAndExpiry(t *testing.T) {
	db := identityDB(t)
	cache := newFakeCache()
	m := identityManager(t, db).WithCache(cache)

	raw := "cache-token" + strings.Repeat("0", 20)
	hashed := models.HashToken(raw)
	db.Create(&models.Session{
		UserID: 1, OrgID: 2, Token: hashed,
		LastSeenAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})

	// A cache-only row (no DB row) proves the cache is consulted.
	cache.Set(context.Background(), "session:"+hashed, &models.Session{
		UserID: 77, OrgID: 78, Token: hashed,
		LastSeenAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}, time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: raw})
	if uid := m.UserID(req); uid != 77 {
		t.Errorf("UserID = %d, want the 77 from the cache row", uid)
	}

	// An expired cached row is evicted and the DB row wins (and is re-cached).
	cached := &models.Session{
		UserID: 88, OrgID: 88, Token: hashed,
		LastSeenAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour),
	}
	cache.Set(context.Background(), "session:"+hashed, cached, time.Minute)
	if uid := m.UserID(req); uid != 1 {
		t.Errorf("UserID after expired cache = %d, want the DB row's 1", uid)
	}
	if v, ok := cache.m["session:"+hashed]; !ok {
		t.Error("expired cached session was evicted but not re-cached from the DB")
	} else if v.(*models.Session).UserID == 88 {
		t.Error("stale cached session survived the expiry check")
	}
}

// TestTouchSessionUpdatesWithinGap forces the interval branch by backdating
// LastSeenAt, and confirms a no-op for a missing cookie.
func TestTouchSessionUpdatesWithinGap(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)

	rec := httptest.NewRecorder()
	m.SetSession(rec, httptest.NewRequest(http.MethodGet, "/", nil), 1, 1)
	raw := sessionCookie(t, rec)
	db.Model(&models.Session{}).Where("token = ?", models.HashToken(raw)).Update("last_seen_at", time.Now().Add(-2*time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: raw})
	m.TouchSession(req)

	var s models.Session
	if err := db.Where("token = ?", models.HashToken(raw)).First(&s).Error; err != nil {
		t.Fatalf("session: %v", err)
	}
	if time.Since(s.LastSeenAt) > time.Minute {
		t.Errorf("LastSeenAt not refreshed: %v", s.LastSeenAt)
	}
}

// TestReporterIP honors proxy headers only when trustProxy is set.
func TestReporterIP(t *testing.T) {
	trustProxy = false
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if got := reporterIP(r); got == "1.2.3.4" {
		t.Error("reporterIP honored X-Forwarded-For without trustProxy")
	}

	trustProxy = true
	t.Cleanup(func() { trustProxy = false })
	if got := reporterIP(r); got != "1.2.3.4" {
		t.Errorf("X-Forwarded-For = %q, want 1.2.3.4", got)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("X-Real-IP", "9.9.9.9")
	if got := reporterIP(r2); got != "9.9.9.9" {
		t.Errorf("X-Real-IP = %q, want 9.9.9.9", got)
	}
}

// TestTwoFAChallengeCookies covers the challenge cookie set/read/clear trio.
func TestTwoFAChallengeCookies(t *testing.T) {
	m := identityManager(t, identityDB(t))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := m.TwoFAChallengeFromRequest(req); got != "" {
		t.Errorf("no cookie read = %q, want empty", got)
	}

	m.SetTwoFAChallengeCookie(rec, req, "challenge-value")
	var found *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == twofaChallengeCookie {
			found = c
		}
	}
	if found == nil || found.Value != "challenge-value" {
		t.Errorf("challenge cookie missing or wrong value: %v, want challenge-value", found)
	}
	if found != nil && found.Path != twofaChallengePath {
		t.Errorf("challenge cookie path = %q, want %q", found.Path, twofaChallengePath)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(found)
	if got := m.TwoFAChallengeFromRequest(req2); got != "challenge-value" {
		t.Errorf("TwoFAChallengeFromRequest = %q, want challenge-value", got)
	}

	rec3 := httptest.NewRecorder()
	m.ClearTwoFAChallengeCookie(rec3, req)
	cleared := false
	for _, c := range rec3.Result().Cookies() {
		if c.Name == twofaChallengeCookie {
			cleared = c.MaxAge == -1 && c.Value == ""
		}
	}
	if !cleared {
		t.Error("challenge cookie was not cleared")
	}
}

// TestMintChallengeRequiresSecret covers the no-secret-key refusal.
func TestMintChallengeRequiresSecret(t *testing.T) {
	m := New(&config.Config{SecretKey: ""}, crypto.New(""))
	if _, err := m.NewTwoFAChallenge(1); err == nil {
		t.Fatal("minted a challenge with no secret key")
		return
	}
}

// TestFirstOrPersonalOrg creates a personal org for a member-less user and
// reuses an existing org for a member, and errors for a missing user.
func TestFirstOrPersonalOrg(t *testing.T) {
	db := identityDB(t)

	user := mustUser(t, db, "alone@acme.com")
	orgID, err := firstOrPersonalOrg(db, user.ID)
	if err != nil {
		t.Fatalf("firstOrPersonalOrg: %v", err)
	}
	if orgID == 0 {
		t.Fatal("no org created")
	}
	var member models.OrgMember
	if err := db.Where("org_id = ? AND user_id = ?", orgID, user.ID).First(&member).Error; err != nil {
		t.Fatalf("creator membership missing: %v", err)
	}
	if member.Role != "owner" {
		t.Errorf("creator role = %q, want owner", member.Role)
	}

	org := mustOrg(t, db, "Existing")
	existing := mustUser(t, db, "existing@acme.com")
	if err := db.Create(&models.OrgMember{OrgID: org.ID, UserID: existing.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("member: %v", err)
	}
	if got, err := firstOrPersonalOrg(db, existing.ID); err != nil || got != org.ID {
		t.Errorf("existing member resolved to %d (err %v), want %d", got, err, org.ID)
	}

	if _, err := firstOrPersonalOrg(db, 424242); err == nil {
		t.Error("missing user resolved to an org")
	}
}

// TestIdentityNormalizeFailures rejects assertions missing parts of the key.
func TestIdentityNormalizeFailures(t *testing.T) {
	for _, id := range []plugin.ExternalIdentity{
		{Issuer: "https://x", Subject: "s"},
		{Provider: "oidc", Subject: "s"},
		{Provider: "oidc", Issuer: "https://x"},
	} {
		if _, err := normalizeIdentity(id); err == nil {
			t.Errorf("normalizeIdentity accepted %+v", id)
		}
	}
	normalized, err := normalizeIdentity(plugin.ExternalIdentity{
		Provider: " OIDC ", Issuer: "https://IdP.example/", Subject: " s ", Email: " A@B.C ",
	})
	if err != nil {
		t.Fatalf("normalizeIdentity: %v", err)
	}
	if normalized.Provider != "oidc" || normalized.Email != "a@b.c" || normalized.Subject != "s" {
		t.Errorf("normalized = %+v", normalized)
	}
}

// TestResolveIdentityNoEmailClaim refuses an unbound identity without an email
// claim before any provisioning happens.
func TestResolveIdentityNoEmailClaim(t *testing.T) {
	db := identityDB(t)
	id := oidcID("https://idp.acme.com", "sub-x", "")
	if _, _, err := resolveIdentity(db, id); err == nil {
		t.Fatal("resolveIdentity accepted an email-less identity")
		return
	}
}

// TestLoginWithoutDatabase refuses login and bind when no DB is configured.
func TestLoginWithoutDatabase(t *testing.T) {
	m := testManager(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id := oidcID("https://idp.acme.com", "s", "a@b.c")
	if _, err := m.LoginByIdentity(rec, req, id); err == nil {
		t.Error("LoginByIdentity without a DB succeeded")
	}
	if _, err := m.LoginByEmail(rec, req, "a@b.c"); err == nil {
		t.Error("LoginByEmail without a DB succeeded")
	}
	if err := m.BindIdentity(req, id); err == nil {
		t.Error("BindIdentity without a DB succeeded")
	}
	if m.userHasTOTP(1) {
		t.Error("userHasTOTP without a DB returned true")
	}
}

// TestOAuthStoreSecure covers the requestSecureStore wrappers: New, Get, Save
// all stamp the per-request Secure flag, and a nil session is a safe no-op.
func TestOAuthStoreSecure(t *testing.T) {
	base := sessions.NewCookieStore([]byte("test-key"))
	store := requestSecureStore{base}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{}

	sess, err := store.New(req, "n")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !sess.Options.Secure {
		t.Error("New did not mark the session Secure for a TLS request")
	}

	got, err := store.Get(req, "n")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Options.Secure {
		t.Error("Get did not mark the session Secure for a TLS request")
	}

	rec := httptest.NewRecorder()
	if err := store.Save(req, rec, sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// nil session must not panic.
	store.markSecure(req, nil)
}

// TestRequestSecureStoreOverHTTP asserts the Secure flag stays off over plain
// HTTP: a storage whose Options.Secure was previously set must not leak it.
func TestRequestSecureStoreOverHTTP(t *testing.T) {
	base := sessions.NewCookieStore([]byte("test-key"))
	base.Options.Secure = true
	store := requestSecureStore{base}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sess, _ := store.New(req, "n")
	if sess.Options.Secure {
		t.Error("plain-HTTP session must not be Secure")
	}
}
