// Sign-in failures that happen *outside* the login form — an email
// verification link, an OIDC round trip, any auth plugin's redirect — cannot
// answer with JSON, because the browser is mid-navigation. They finish the trip
// at /admin/login?error=<code>, and this maps that code to what the page says.
//
// The vocabulary is deliberately generic. Core owns the login screen while auth
// methods are pluggable, so a plugin must be able to explain itself without
// core knowing the plugin exists: "the provider is unreachable" is core
// vocabulary, "SSO discovery failed against issuer X" is not.
//
// Keep it a closed set. An unrecognised code falls back to the generic message
// rather than being echoed — the code arrives in a URL anyone can edit, and
// rendering it verbatim would paint attacker-chosen text onto the login page.

const AUTH_ERROR_KEYS: Record<string, string> = {
  invalid_token: "app.authError.invalidToken",
  expired_token: "app.authError.expiredToken",
  provider_unavailable: "app.authError.providerUnavailable",
  provider_error: "app.authError.providerError",
  session_expired: "app.authError.sessionExpired",
  email_unverified: "app.authError.emailUnverified",
  domain_not_allowed: "app.authError.domainNotAllowed",
  invite_only: "app.authError.inviteOnly",
  account_link_required: "app.authError.accountLinkRequired",
  login_failed: "app.authError.loginFailed",
};

/** The i18n key for a redirect's ?error= code. Unknown codes get the generic one. */
export function authErrorKey(code: string | null | undefined): string | null {
  if (!code) return null;
  return AUTH_ERROR_KEYS[code] ?? AUTH_ERROR_KEYS.login_failed;
}

/** Every code the login page understands — used by tests and by callers. */
export function authErrorCodes(): string[] {
  return Object.keys(AUTH_ERROR_KEYS);
}

/**
 * True when the sign-in attempt succeeded at verifying an email address.
 *
 * Accepts both spellings: the server has always sent `?verified=1` while this
 * page only ever checked for `"true"`, so the success banner never appeared
 * once. Reading both means fixing it here does not depend on every redirect
 * being updated in the same breath.
 */
export function isVerifiedFlag(value: string | null | undefined): boolean {
  return value === "1" || value === "true";
}
