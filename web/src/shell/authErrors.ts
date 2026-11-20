
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

export function authErrorKey(code: string | null | undefined): string | null {
  if (!code) return null;
  return AUTH_ERROR_KEYS[code] ?? AUTH_ERROR_KEYS.login_failed;
}

export function authErrorCodes(): string[] {
  return Object.keys(AUTH_ERROR_KEYS);
}

export function isVerifiedFlag(value: string | null | undefined): boolean {
  return value === "1" || value === "true";
}
