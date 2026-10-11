# Engineering Specification: `internal/api`

## 1. Overview & Architectural Role

`server/internal/api` implements the JSON HTTP REST API for Octarq, built on the `danielgtaylor/huma/v2` framework mounted on Go's standard `http.ServeMux`. It serves as the primary gateway for browser clients, machine integrations, and administrative dashboards.

The module manages:
- **Authentication & Sessions**: Email/password, OAuth providers, session cookies, bearer token minting/revocation, and 2FA.
- **Tenant & Workspace Scoping**: Org membership, role hierarchies (`owner` > `admin` > `member`), workspace switching, and isolated data queries.
- **Settings & Dynamic Toggles**: Global instance settings, workspace settings, white-label branding, and plugin feature gates.
- **Notification Routing**: In-app notifications, user delivery preferences, and outbound channel management (Email, Webhook, Chatbot).
- **Abuse Reporting & Audit Logging**: Cryptographically verifiable audit trails and rate-limited abuse ingress.

---

## 2. Invariants & Security Boundaries

### 2.1 Uniform Error Envelope Contract
Every HTTP error returned by the API—whether triggered by validation failure, auth rejection, rate limiting, or internal panics—strictly adheres to the single, documented envelope (`apierror.Error`):

```json
{
  "code": "unauthorized",
  "message": "unauthorized",
  "details": [],
  "request_id": "c1f7a8b9..."
}
```

- **Stable Codes**: Clients branch on `code`, not human-readable `message`.
- **Status Normalization**: Huma's request validation status `422 Unprocessable Entity` is normalized to `400 Bad Request` across the API surface.

### 2.2 Fail-Closed Tenant Context & Role Isolation
- **Org Scoping (`requireOrg`)**: A handler accessing workspace data must resolve an active `orgID > 0`. If unauthenticated or no active organization is bound, the handler strictly fails closed (`401 Unauthorized` or `403 Forbidden`).
- **Owner Role Invariant**: An organization must never be left without an active owner. Demoting or removing the final owner of a workspace is rejected with `400 Bad Request`.
- **Session Revocation Invariant**: Any modification of a member's role (promotion, demotion, or removal) immediately purges all stateful session rows (`models.Session`) and invalidates cache keys (`session:<token>`) for that user in that workspace.

### 2.3 Rate Limiter Budget Isolation
To prevent Denial of Service and credential stuffing, rate limits are enforced via `ulule/limiter/v3` across isolated budgets:
1. **Login Budget (`loginLimiter`)**: 5 failed login attempts per 15 minutes per IP.
2. **Recovery Budget (`recoveryLimiter`)**: 5 reset/verification requests per 15 minutes per IP (deliberately independent of login).
3. **Register Budget (`registerLimiter`)**: 5 public sign-ups per hour per IP.
4. **Abuse Budget (`abuseLimiter`)**: 5 abuse reports per hour per IP.
5. **Send Budget (`sendLimiter`)**: 100 outbound emails per organization per hour.
6. **Subsystem Status Budget (`statusLimiter`)**: 60 status checks per minute per IP.

---

## 3. Module File Architecture & God File Ceiling

All production code files in `internal/api` strictly adhere to the **< 500 LOC** hard limit:

| File | Purpose | LOC |
|---|---|---|
| `api.go` | `Handler` struct definition, dependency injection, and helper utilities | ~204 |
| `api_routes.go` | Route mounting on `http.ServeMux`, Huma registration, and OpenAPI descriptions | ~419 |
| `auth_session.go` | Session listing, revocation, current user `/api/auth/me`, and global logout | ~291 |
| `auth_email.go` | Authenticated email change workflows and verification token issuance | ~150 |
| `auth_invite.go` | Workspace invite acceptance, password onboarding, and session initiation | ~111 |
| `tenant_members.go` | Workspace member listing, role promotion/demotion, and removal guards | ~471 |
| `tenant_members_invite.go` | Workspace invite dispatch, re-generation, and cooldown rate limits | ~135 |
| `tenant_menu.go` | Organization CRUD, workspace address allocation, and slug switching | ~290 |
| `tenant_navigation.go` | Navigation menu generation, global action providers, and user preferences | ~253 |
| `settings.go` | Setting keys, retention configuration, rate-limit accessors, and branding | ~297 |
| `settings_workspace.go` | Workspace settings REST endpoints and inbound webhook token management | ~238 |
| `plugins.go` | Workspace plugin feature enablement, cascade toggling, and dependency resolution | ~453 |
| `plugins_instance.go` | Instance administrator plugin registry and metadata reporting | ~88 |
| `notify_channels.go` | Notification channel CRUD, router dispatcher, and test delivery | ~454 |
| `notify_channels_crypto.go` | AES-GCM config encryption/decryption and secret redaction | ~88 |

---

## 4. Verification & Testing

- **Characterization Tests**: [`api_characterization_test.go`](file:///Volumes/PHD/code/.worktrees/octarq-refine-api/server/internal/api/api_characterization_test.go) locks public vs protected route boundaries, rate limit defaults, and OpenAPI schema compliance.
- **Race Detector**: All tests must execute cleanly under `go test -race ./internal/api/...`.
