
export function oauthBeginPath(provider: string): string {
  return `/auth/begin/${provider}`;
}

export function oauthCallbackPath(provider: string): string {
  return `/auth/callback/${provider}`;
}
