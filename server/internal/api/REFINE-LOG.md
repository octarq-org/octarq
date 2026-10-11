# Refinement Log: `internal/api`

## Date: 2026-10-04
## Branch: `refine/pd-api`
## Status: 🟢 Refined (P0)

---

### 1. Invariants Locked
- Added `api_characterization_test.go` locking:
  - Public vs protected endpoint boundaries (`/api/health` reachable anonymously, private routes reject with 401 and valid JSON error envelope).
  - Rate limiter budget independence (`login`, `recovery`, `register`, `abuse`).
  - OpenAPI/Huma route registration integrity (`paths`, `title`, `version`).
  - Strict fail-closed semantics for workspace setting mutations when `orgID=0`.

---

### 2. God Files Decomposed (< 500 LOC Hard Limit)
Prior to refinement, 7 production Go files exceeded 500 lines. All 7 were decomposed into focused, single-responsibility files:

1. `api.go` (616 $\to$ 204 LOC):
   - Extracted `Routes()` and OpenAPI front-matter into `api_routes.go` (419 LOC).
2. `tenant_members.go` (588 $\to$ 471 LOC):
   - Extracted invite delivery and invite resending into `tenant_members_invite.go` (135 LOC).
3. `notify_channels.go` (537 $\to$ 454 LOC):
   - Extracted AES-GCM config encryption/decryption and secret redaction into `notify_channels_crypto.go` (88 LOC).
4. `tenant_menu.go` (533 $\to$ 290 LOC):
   - Extracted menu aggregation, plugin actions, and user settings into `tenant_navigation.go` (253 LOC).
5. `plugins.go` (531 $\to$ 453 LOC):
   - Extracted instance plugin inventory into `plugins_instance.go` (88 LOC).
6. `settings.go` (530 $\to$ 297 LOC):
   - Extracted workspace settings endpoints and inbound token rotation into `settings_workspace.go` (238 LOC).
7. `auth_session.go` (521 $\to$ 291 LOC):
   - Extracted authenticated email change into `auth_email.go` (150 LOC).
   - Extracted invite acceptance into `auth_invite.go` (111 LOC).

**Result**: 100% of production Go files in `internal/api` are now strictly < 500 lines.

---

### 3. Diagnostic & Fail-Closed Fixes
- **Entropy Exhaustion Fail-Closed (`webhooks.go:128`)**: Replaced swallowed `_, _ = rand.Read(b)` with strict error check returning 500 on entropy failure.
- **Abuse Queue & Notification Logging (`abuse.go:104, 127`)**: Swallowed errors replaced with structured `slog.Warn` diagnostics for report tracing.
- **Stream Response Logging (`files.go:340`, `settings_backup.go:67`)**: Added error checking and warning logging on stream download completion/disconnection.
- **Temp Backup File Cleanup (`settings_backup.go:56, 71`)**: Added error checking and warning logging on temporary backup file removal.
- **Session Cache Deletion Warnings**: Swallowed cache deletions in `auth_password.go:109`, `account.go:363`, `recovery.go:264`, `tenant_members.go:208, 351`, and `auth_session.go:53, 194` converted to structured warnings.
- **Rate Limiter Store Diagnostics (`ratelimit.go:64, 69`)**: Logged store failure warnings on increment and reset.

---

### 4. Verification Results
- `go test -race ./internal/api/...`: **100% PASS** (40.8s)
- `go build ./...`: **100% PASS**
- `gofmt -w .`: **CLEAN**
