// The two OAuth paths the Go half serves, in one place.
//
// These are registered in internal/api/api.go as
//   GET /auth/begin/{provider}
//   GET /auth/callback/{provider}
// and internal/auth/oauth.go builds the redirect URI it sends the provider from
// the same shape (callbackBase + "/auth/callback/" + provider).
//
// They live here because the second one had already drifted: the settings page
// displayed "/api/auth/google/callback" as the URL to register with Google,
// which is neither the right prefix nor the right order. Nothing caught it,
// because the frontend never *calls* the callback URL — the provider does,
// after an operator pastes it into a console we can't see. The failure surfaces
// as "OAuth just doesn't work", far from this screen.
//
// oauthRoutes.test.ts asserts both against the Go source, so a route rename on
// that side fails here instead of shipping a 404 into someone's provider config.

export function oauthBeginPath(provider: string): string {
  return `/auth/begin/${provider}`;
}

export function oauthCallbackPath(provider: string): string {
  return `/auth/callback/${provider}`;
}
