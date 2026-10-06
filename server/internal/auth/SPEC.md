# SPEC: Authentication & Session Management (`server/internal/auth`)

## 1. Overview & Architecture
`server/internal/auth` provides core session authentication, token-based API authentication, multi-tenant context binding, OAuth 2.0 flow handling, and two-factor challenge workflows for Octarq Core.
- **Stateful Sessions**: DB-backed sessions with SHA-256 token hashing (`models.Session`). The raw token is stored in an HTTP-only, SameSite-Lax signed cookie. No raw session tokens are stored in the database or cache.
- **API Bearer Tokens**: Stateless API access using `oct_...` bearer tokens, validated via SHA-256 hash lookup against `models.Token`.
- **Double Submit CSRF**: Automatically synchronizes paired CSRF tokens derived from the session token.
- **Multi-Tenant Context Binding**: Injects `userID`, `orgID`, `sessionID`, and token role ceiling into `context.Context` via `plugin.WithOrgID` and `tenantsql.WithUserID`.
- **Caching Layer**: Optional Redis/in-memory cache for fast session lookup with automatic eviction on session updates, touch, revocation, and logout.

## 2. Exported Contracts & Types
- `Manager`: Main session and auth orchestrator.
  - `New(cfg *config.Config, c *crypto.Cipher) *Manager`
  - `WithDB(db *gorm.DB) *Manager`
  - `WithCache(c cache.Cache) *Manager`
  - `SetSessionFromRequest(r *http.Request, w http.ResponseWriter, uid, orgID uint)`
  - `SetSession(w http.ResponseWriter, r *http.Request, uid, orgID uint)`
  - `Authed(r *http.Request) bool`
  - `APIAuthed(r *http.Request) bool`
  - `UserID(r *http.Request) uint`, `OrgID(r *http.Request) uint`, `SessionID(r *http.Request) uint`
  - `TouchSession(r *http.Request)`
  - `Clear(r *http.Request, w http.ResponseWriter)`
  - `RevokeUserOrgSessions(userID, orgID uint) int`
  - `RevokeUserOrgTokens(userID, orgID uint) int`
  - `Require(next http.Handler) http.Handler`
  - `AuthenticateRequest(r *http.Request) (*http.Request, bool)`
- Context Helpers:
  - `WithOrgID(ctx context.Context, orgID uint) context.Context`
  - `WithUserID(ctx context.Context, uid uint) context.Context`
  - `UserIDFromContext(ctx context.Context) uint`
  - `WithTokenID(ctx context.Context, id uint) context.Context`
  - `TokenIDFromContext(ctx context.Context) uint`
  - `WithTokenRole(ctx context.Context, role string) context.Context`
  - `TokenRoleFromContext(ctx context.Context) string`

## 3. Reliability & Security Guarantees
- **Fail-Closed Session Creation**: Database session creation errors are logged with `slog.Error` and fail closed, preventing orphaned cookies.
- **Fail-Closed Random Entropy**: Session token generation relies on `crypto/rand.Read` and panics fail-closed if system entropy fails.
- **Constant-Time Verification**: Admin username and password verification uses `crypto/subtle.ConstantTimeCompare` to avoid timing side channels.
- **Secure Cookie derivation**: Derives cookie TLS security dynamically from `origin.Secure(r, trustProxy)` rather than static flags, preventing browser drops over HTTP while maintaining strict TLS flags over HTTPS.
- **Strict Size Ceiling**: Every source and test file is strictly bounded under 500 lines of code.

## 4. File Layout
- `auth.go`: `Manager` definition, session lifecycle, cookie management, middleware (`Require`, `AuthenticateRequest`). (< 400 LOC)
- `auth_context.go`: Context keys, context injection/extraction helpers, client IP resolution, constant-time admin checks. (< 150 LOC)
- `auth_token.go`: Bearer token resolution, role ceiling propagation, last-used timestamp updates, and token revocation. (< 150 LOC)
- `identity.go`: User identity resolution, password validation, user lookup.
- `methods.go`: Available authentication method declarations and inspection.
- `oauth.go`: OAuth 2.0 provider integration, state validation, callback handling.
- `provision.go`: Automatic provisioning for authenticated identities.
- `twofa_challenge.go`: Two-factor authentication challenge issuance and verification.
- `auth_characterization_test.go`: Characterization test locking session lifecycle, context propagation, and revocation contracts.
