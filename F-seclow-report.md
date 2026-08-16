# F-seclow report — security audit remaining three items

Branch: `fix/seclow`. Changes confined to `internal/models/` and `internal/api/`
(plus `internal/api/*_test.go`). Nothing under `internal/server/`, `main.go`,
`config/`, `web/`, `docs/`, `README*` was touched.

Verification (all green):

- `gofmt -l .` → no output
- `go build ./...` → ok
- `go vet ./...` → ok
- `go test ./... -race -count=1` → ok (35 packages)

Timing note: on this machine the `internal/api` package alone takes ~613s under
`-race`, marginally over Go's default 600s per-package alarm, so the final full
run used `-timeout 25m`. No timeout was shortened; the suite passes under race.

---

## L-1 — invite token was plaintext, unindexed

### Change

Mirrored the existing reset/verify token pattern exactly (SHA-256 hex + index),
no third invention.

- `internal/models/models.go:53` — `User.InviteToken string gorm:"size:255"`
  replaced with `InviteTokenHash string gorm:"index;size:64"` (column
  `invite_token_hash`), the same shape as `ResetTokenHash` / `VerifyTokenHash`.
- `internal/api/tenant_menu.go:414` — `addOrgMember` stores
  `InviteTokenHash: hashToken(rawInviteToken)`; the raw 192-bit token lives only
  in the local `rawInviteToken` variable, returned once in the API response and
  embedded in the emailed accept link (`inviteUrl` / `sendInviteEmail`).
- `internal/api/auth.go:837` — `acceptInvite` looks up by
  `Where("invite_token_hash = ?", hashToken(token))` and clears the hash on
  redemption (`auth.go:851`).
- `internal/api/tenant_menu.go:312,328` — `listOrgMembers` selects
  `users.invite_token_hash` and derives `Pending` from it; the member list only
  needs presence, never the raw token.

### Schema migration

`invite_token_hash` is created by the existing single delayed AutoMigrate pass
(`app/preflight.go` is the preflight collision guard; the migration itself runs
in `app.Run` over `models.AllModels()` — the `User` model is part of it). GORM
AutoMigrate adds the new column and its index on an existing DB and leaves the
now-unused `invite_token` column in place; there are no legacy deployments to
migrate data for, so no backfill/rollback path was written. Tests migrate a
fresh schema per handler, so the new column is exercised everywhere.

## L-3 — the two "completion" endpoints had no rate limit

### Change

Reused the existing `recoveryLimiter` (5 / 15 min per IP, the same budget
`forgotPassword` and `resendVerification` already spend); no third limiter was
introduced.

- `internal/api/recovery.go:197-200` — `resetPassword` now unwraps the request,
  `allow`s on `recoveryLimiter` (429 on exhaustion) and counts every request,
  exactly like `forgotPassword` (same rationale comment; there is no "failed"
  attempt to budget on for an unauthenticated completion endpoint).
- `internal/api/auth.go:822-825` — `acceptInvite` gets the same gate, same
  budget.

Both endpoints are public (`internal/api/public_endpoints.go`), so unauthenticated
requesters share the same per-IP budget as the reset initiation endpoints.

## L-8 — sensitive operations left no audit trail

### Change

All new audit writes use the existing `h.audit` / `h.auditAs` helpers
(asynchronous, never blocking). New events follow the `noun.verb` convention.

| Event | Where | Notes |
|---|---|---|
| `user.change_password` | `internal/api/auth.go:365` | after the hash write + other-session revocation; meta carries only `sessionsRevoked` |
| `user.logout` | `internal/api/auth.go:605` | actor/org/session captured before `Clear`; target = the session row |
| `user.logout_all` | `internal/api/auth.go:259` | meta carries `sessionsRevoked` |
| `user.session_revoke` | `internal/api/auth.go:531` | target = the revoked session |
| `user.switch_org` | `internal/api/tenant_menu.go:97` | row written to the org switched INTO; meta carries `{from, to}` |
| `auth.login_failed` | `internal/api/auth.go:898` | written in `publishLoginFailed`, once per org the account belongs to (same fan-out as the existing eventbus event) |

Failed logins: `publishLoginFailed` previously only published the eventbus
event; it now also records `auth.login_failed` in `audit_logs`. Meta is
deliberately limited to `{email, reason}` — **no password, no token** — matching
the eventbus payload.

The `account.go` purge exception was left untouched, per the audit's explicit
instruction: `purgeAccount` deliberately writes no audit row (comment at
`internal/api/account.go:245-251` explains why — the row would be resurrected
data for an org being erased, and the write would race the delete).

---

## Guard tests (all in `internal/api/seclow_guard_test.go`)

1. `TestInviteTokenStoredHashedAndRawStillAccepts` — DB stores the SHA-256 hash
   (≠ raw token, == `hashToken(raw)`), and POSTing the raw token still redeems
   the invite (200, hash cleared, password set).
2. `TestResetPasswordRateLimited` — five `/api/auth/reset` calls from one IP are
   processed; the sixth answers 429.
3. `TestChangePasswordAuditsWithoutStoringPassword` — a successful change lands
   `user.change_password` in `audit_logs` with the correct actor and a meta that
   contains neither the old nor the new password.

Existing invite tests were updated for the schema (`internal/api/invite_dns_test.go`
now asserts the DB holds the hash of the response token, and that the hash is
cleared on accept).

## Mutation verification (product guard short-circuited → test turns red → reverted)

1. `internal/api/tenant_menu.go:414` — `InviteTokenHash: hashToken(rawInviteToken)`
   mutated to store the raw token → `TestInviteTokenStoredHashedAndRawStillAccepts`
   red: `seclow_guard_test.go:50 "invite token stored in plaintext: DB value equals the raw token"`.
2. `internal/api/recovery.go:197` — `if !h.recoveryLimiter.allow(ip)` mutated to
   `if false && !h.recoveryLimiter.allow(ip)` → `TestResetPasswordRateLimited` red:
   `seclow_guard_test.go:83 "attempt 6: got 400 (...), want 429"`.
3. `internal/api/auth.go:365` — `h.audit(r, "user.change_password", ...)` wrapped
   in `if false { ... }` → `TestChangePasswordAuditsWithoutStoringPassword` red:
   `seclow_guard_test.go:106 "audit row for action \"user.change_password\" never appeared within deadline"`.

All mutations compiled and were reverted afterwards; `grep -rn MUTATION internal/`
returns nothing.

## Files changed

- `internal/models/models.go`
- `internal/api/auth.go`
- `internal/api/recovery.go`
- `internal/api/tenant_menu.go`
- `internal/api/invite_dns_test.go`
- `internal/api/seclow_guard_test.go` (new)
