# FIX-auth report

Branch: `fix/ship-auth` · Worktree: `.worktrees/octarq-ship-auth`

Three audited auth dead-ends fixed: (1) fresh instances could never deliver
the verification email their own sign-up promised, (2) invited teammates were
blocked from login by a verification gate they could never pass, (3) the
public-registration rate limit never counted anything.

---

## Problem 1 — sign-up dead end on instances that cannot send mail

**Root cause:** `registrationEnabled()` and `requireEmailVerification()` both
default **on**, but nothing checked whether the instance could actually deliver
the verification email. `sendVerificationEmail` is best-effort (`log.Printf` on
failure), and `sendMail` returns `"no SMTP sender configured for org %d"` when
no sender exists — so a fresh `docker run` accepted sign-ups, never sent the
mail, and then 403'd the login forever.

**Fix — the system now tells the truth:**

1. **New service contract (mail plugin side):**
   - `plugin/services.go`:
     - `const ServiceMailReady = "mail.ready"`
     - `type MailReady func() bool` — true when at least one SMTP sender is
       configured anywhere on the instance. Consumers treat "service absent"
       as not-ready (fail closed).
   - `plugins/mail/plugin.go`:
     - `func (p *Plugin) mailReady() bool` — counts `SMTPSender` rows; `> 0`
       means ready. Provided in `Mount` as `plugin.MailReady(p.mailReady)`
       under `plugin.ServiceMailReady`, with the compile-time contract
       assertion `_ plugin.MailReady = (*Plugin)(nil).mailReady` alongside the
       existing `MailSender`/`EmailDispatcher` assertions.
2. **Core consumer:** `internal/api/recovery.go` — `func (h *Handler) mailReady() bool`
   resolves the service via `plugin.LookupServiceAs[plugin.MailReady](h.LookupService, plugin.ServiceMailReady)`
   (same pattern as `sendVerificationEmail` at `recovery.go:65`); no bare `any`.
3. **Register endpoint:** `internal/api/register.go:83` — when
   `requireEmailVerification()` is true but the instance cannot send mail,
   `POST /api/auth/register` returns **503** with
   `"this instance cannot send email yet; ask the administrator to configure an SMTP sender or disable email verification"`
   instead of an unfulfillable `verificationRequired`. The check runs before
   any user/org row is created, so no dead account lingers.
4. **Startup readiness:** `app/readiness.go` — the report's "outbound mail"
   line is now driven by the **mail.ready** service (a real sender state), not
   by the mail.send service merely existing. `app/app.go` resolves it after the
   mount loop:
   `plugin.LookupServiceAs[plugin.MailReady](services.Lookup, plugin.ServiceMailReady)`.
   Degraded copy tells the operator the exact remedy; the OK copy now reads
   "at least one SMTP sender is configured". `TestReadinessReportOmitsSecrets`
   and the degraded-line "mail.send" action hint still pass unchanged.
5. **Frontend:** the 503 detail is user-visible, so `web/src/shell/Login.tsx`
   detects it (`cannot send email`) and renders the localized
   `app.registerMailUnavailable` string instead of the raw English API message
   (same pattern as the existing `isUnverifiedErr` match).

## Problem 2 — invite acceptance never marked the email verified

**Root cause:** `acceptInvite` (`internal/api/auth.go`) set `PasswordHash` and
cleared `InviteToken`/`InviteExpiresAt` but never wrote `EmailVerified`; with
the default-on gate, the just-invited teammate was 403'd at login.

**Fix:** `internal/api/auth.go:784` — `user.EmailVerified = true` on
successful redemption, with a comment explaining why it is safe: the invite
token is delivered only to the account's own mailbox (`sendInviteEmail`), so
redeeming a valid, unexpired token proves ownership of that address.

## Problem 3 — registration rate limit was a no-op

**Root cause:** `register.go` used the shared `loginLimiter`, whose `allow()`
only `Peek()`s (never counts) and whose counts come exclusively from
`recordFailure` — and registration **success** called `loginLimiter.reset(ip)`,
zeroing even those. Net effect: unlimited sign-ups per IP.

**Fix:**
- `internal/api/api.go` — new `registerLimiter *rateLimiter` field, wired in
  `New` as `newRateLimiter(cfg.RedisURL, "register", 5, time.Hour)` (5 sign-ups
  per IP per hour, same order as `recoveryLimiter`).
- `internal/api/register.go:76-79` — register now checks **and counts** every
  request, copying the `recoveryLimiter` pattern (`allow` → `recordFailure`).
- `internal/api/register.go` — the success path no longer calls
  `h.loginLimiter.reset(ip)`; login's failure budget is untouched by sign-ups
  and vice versa.

## Service contract signatures (new)

```go
// plugin/services.go
const ServiceMailReady = "mail.ready"
type MailReady func() bool

// plugins/mail/plugin.go (provider)
func (p *Plugin) mailReady() bool            // counts SMTPSender rows, >0 = ready

// internal/api/recovery.go (consumer)
func (h *Handler) mailReady() bool           // LookupServiceAs[plugin.MailReady]
```

## Guard tests

| Test | File | What it pins |
|---|---|---|
| `TestInviteAcceptMarksEmailVerifiedAndAllowsLogin` | `internal/api/invite_dns_test.go` | invite redemption sets `EmailVerified`, and the user logs in under the (explicitly on) verification gate |
| `TestRegisterFailsLoudlyWhenMailUnavailable` | `internal/api/register_test.go` | gate on + no SMTP sender → 503 explaining the failure, no account created |
| `TestRegisterRateLimitedPerIP` | `internal/api/register_test.go` | 5 registrations from one IP succeed, the 6th gets 429 |

Existing tests adapted to the new contract: `TestRegisterWithVerificationGateGivesNoSession`
seeds one `SMTPSender` so it exercises the `verificationRequired` branch (the
no-mail branch is now a 503); `TestRegisterRejectsDuplicateAndShortPassword`,
`TestRegisterUsesOrgNameOverEmail` and the six recovery-flow tests
(`TestForgotPasswordFailsLoudlyWhenTokenWriteFails`,
`TestVerifyEmailRedirectsWithErrorWhenWriteFails`,
`TestResendVerificationFailsLoudlyWhenTokenWriteFails`, `TestForgotPasswordNoLeak`,
`TestVerifyEmailAndResendFlow`, `TestForgotPasswordNoLeakExistence`) opt out of
the gate via the documented `disableEmailVerification` helper, since a
mail-less test instance can no longer pass it and register is pure setup for
what those tests exercise.

### Mutation verification (each guard short-circuited with a still-compiling `&& false`, test run red, then reverted)

| Guard | Mutated at | Test that went red |
|---|---|---|
| `user.EmailVerified = true` in acceptInvite | `internal/api/auth.go:784` (`if true && false { … }`) | `TestInviteAcceptMarksEmailVerifiedAndAllowsLogin` → `invite_dns_test.go:204 "acceptInvite did not mark the invited email verified"` |
| register 503 mail-unavailable check | `internal/api/register.go:83` (`&& !h.mailReady() && false`) | `TestRegisterFailsLoudlyWhenMailUnavailable` → `register_test.go:248` got 200 `verificationRequired`, want 503 |
| register rate-limit check | `internal/api/register.go:76` (`&& false` after `allow(ip)`) | `TestRegisterRateLimitedPerIP` → `register_test.go:280` 6th register got 200, want 429 |

All three mutations were reverted; the guards are back in place and the tests
are green again.

## New i18n keys (all five dictionaries)

One key added to the `app` namespace in `web/src/i18n/{en,zh,es,pt,ja}.ts`:

- `app.registerMailUnavailable`

(en) "This instance cannot send email yet, so accounts cannot be verified. Ask
an administrator to configure an SMTP sender or disable email verification."

## Verification (as run)

```
gofmt -l .            → no output
go build ./...        → ok
go vet ./...          → ok
go test ./... -race   → ok, all 35 packages (internal/api 438s under the
                       race detector)
cd web
pnpm exec tsc --noEmit → ok
pnpm test             → 26 files / 96 tests passed
pnpm i18n:audit       → all checks passed
```

Note: while another session's concurrent `go test ./... -race` was running,
the frontend suite showed 3 unrelated flakes (`App.test.tsx`,
`brandRefresh.test.tsx`, `tenantSubdomain.test.tsx`) that all pass on a clean
HEAD checkout and pass again on this branch once the race run no longer
starved the CPU.
