# Endpoints reachable without a session

Everything under `/api/` requires a dashboard session. Two mechanisms punch holes
in that, both in the auth middleware in `internal/api/api.go`:

1. **`Metadata["public"] = true`** on a `huma.Operation`. Any plugin can set it,
   including plugins outside this repository.
2. **A path in `publicExactPaths`, or under a `publicSubtreePrefixes` entry**
   (`internal/api/public_endpoints.go`).

Both are necessary. Login cannot require a session; a payment webhook arrives
with no cookie. What was missing was an inventory — a route could become
anonymously reachable and nothing anywhere recorded it, so review depended on
whoever happened to read the diff.

`TestPublicEndpointRegistry` closes that. It enumerates the registered
operations, works out which ones the middleware waves through, and compares
against the reviewed list in `internal/api/public_endpoints_test.go`. A new
public route fails the build until someone writes down why it is safe.

**When that test fails, do not paste the failure output into the list.** Open the
handler and establish how it authenticates its own caller. If it doesn't, the
route is an authentication bypass and the route is what needs fixing.

## The current set

Every entry below was read against its handler.

| Endpoint | Exempt via | What authenticates the caller |
| :-- | :-- | :-- |
| `POST /abuse` | outside `/api/` | Public by design — a takedown request comes from a stranger. It writes a row, so it is rate-limited as its own tier (`internal/server/middleware.go:205`). |
| `POST /api/auth/login` | exact path | Credentials plus optional TOTP, on the auth rate-limit tier. |
| `POST /api/auth/register` | exact path | Gated by the instance's registration setting. |
| `POST /api/auth/2fa/verify` | exact path | Consumes a pending 2FA challenge. |
| `POST /api/auth/logout` | exact path | Clears the caller's own cookie; nothing else. |
| `GET /api/auth/config` | exact path | Instance configuration. No tenant data. |
| `GET /api/auth/methods` | metadata | Which login methods are enabled. No tenant data. |
| `POST /api/auth/invite/accept` | exact path | Single-use hashed invite token. |
| `POST /api/auth/forgot` | metadata | Always answers 200, so it cannot enumerate accounts. |
| `POST /api/auth/reset` | metadata | Single-use hashed reset token with an expiry. |
| `GET /api/auth/verify-email` | metadata | Single-use hashed verification token. |
| `POST /api/auth/resend-verification` | metadata | Always answers 200, so it cannot enumerate accounts. |
| `GET`/`POST /api/dns/ddns/update` | metadata | Hashed per-record DDNS token (`plugins/dns/ddns.go`). A router calls this on a schedule with no cookie; the token is the whole authentication. |
| `POST /api/webhook/{orgSlug}/email/inbound/{token}` | metadata | The org's inbound token as a path segment, constant-time compared. |
| `POST /api/webhook/{orgSlug}/email/inbound/raw/{token}` | metadata | Same token, same place. This route exists for providers whose entire configuration surface is a URL field (SendGrid Inbound Parse, Mailgun routes), so the credential has to travel in the URL. |
| `POST /api/webhook/{orgSlug}/email/bounce/{token}` | metadata | Same token, plus `isAWSSNSURL` validating the SNS `SubscribeURL` host so a confirmation cannot point the server at an internal address. |
| `GET /api/health`, `GET /api/status` | exact path / metadata | Liveness. No tenant data, no side effects. |

## What the audit found

**`/api/auth/logout-all` was exempt by accident.** The gate matched
`/api/auth/logout` as a *prefix*, and `strings.HasPrefix` matches
`/api/auth/logout-all` too — an endpoint that deletes every session a user
holds. It was never exploitable: `logoutAll` re-authenticates on its own
(`internal/api/auth.go:189`) and returns 401 without a session. But the gate was
not the reason it was safe. The handler author's care was, and that is precisely
the arrangement an inventory exists to stop depending on.

Exact matching replaced prefix matching for every entry except the one that
genuinely needs a subtree. A new endpoint can no longer inherit an exemption by
sharing a name with an old one. `TestLogoutAllIsNotExemptByPrefix` pins it.

## Why the subtree stays

`/api/webhook/` has to match a subtree: the concrete path carries the workspace
slug and a per-mailbox token as segments, so it is never known ahead of time.
That makes every route mounted beneath it public by construction — including
ones nobody has written yet.

The registry test is what makes that survivable. It lists *concrete operations*,
not prefixes, so a new webhook route appears as a diff on the reviewed list
rather than quietly inheriting the exemption. `TestPublicPrefixesAreNarrow`
keeps a second subtree from being added without the same argument being made.

## The inbound token

All three mail webhooks authenticate on one secret: `Org.InboundToken`, a UUID
carried as a path segment. It is org-wide, not per-mailbox — an earlier draft of
this document said per-mailbox and was wrong.

**A credential in a URL is a deliberate choice here, not an oversight.** These
routes are called by mail providers whose entire configuration surface is a
single URL field; a required header would leave SendGrid Inbound Parse and
Mailgun routes unable to call them at all. It is the same shape n8n and every
hosted webhook receiver uses, and it is workable for the same reason: the token
rotates from Mail settings (submitting an empty `inboundToken` mints a fresh
UUID), so a leak costs one repasted URL rather than a migration.

What the URL placement does cost is confidentiality in transit-adjacent places —
access logs, proxy logs, error reports. Two things offset it:

- **Rejections are audited.** A wrong token writes `email.inbound.auth_failed`
  with the route and source IP, and never the attempted token itself. Guessing
  the org token buys the ability to inject mail into any mailbox in the
  workspace, so a silent 401 meant someone could grind the space indefinitely
  with nothing to notice.
- **`/api/webhook/` sits in the strict auth rate-limit tier** (`tierFor` in
  `internal/server/middleware.go`), not the generous API tier.

Header-borne tokens are no longer accepted on any of these routes. Supporting
both would mean two credential paths to audit and one of them unused.
