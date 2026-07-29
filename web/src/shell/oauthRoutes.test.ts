import { describe, it, expect } from "vitest";
import { oauthBeginPath, oauthCallbackPath } from "./oauthRoutes";

// These two paths are a contract with the Go half, and the settings page shows
// the callback one to operators as the redirect URI to register with Google or
// GitHub. It was wrong — "/api/auth/google/callback" against a real route of
// "/auth/callback/google" — and nothing could catch it: the frontend never
// calls that URL, the provider does, so the only symptom is that OAuth sign-in
// silently never works for anyone who followed the instructions on screen.
//
// So the assertion reads the Go route table instead of restating the paths,
// which would just be the same copy that drifted the first time.
const GO_ROUTES = import.meta.glob("../../../internal/api/api.go", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const ROUTE_PATTERN = /mux\.HandleFunc\("GET (\/auth\/(?:begin|callback)\/\{provider\})"/g;

function goOAuthRoutes(): string[] {
  const out: string[] = [];
  for (const code of Object.values(GO_ROUTES)) {
    for (const m of code.matchAll(ROUTE_PATTERN)) out.push(m[1]);
  }
  return out;
}

describe("OAuth route contract with the Go half", () => {
  it("parses both routes out of internal/api/api.go", () => {
    // Without this the two assertions below pass vacuously against an empty
    // list — the guard would go quiet exactly when the routes were renamed.
    expect(goOAuthRoutes().sort()).toEqual(["/auth/begin/{provider}", "/auth/callback/{provider}"]);
  });

  it("builds paths the backend actually serves", () => {
    const routes = goOAuthRoutes();
    for (const provider of ["google", "github"]) {
      expect(routes).toContain(oauthBeginPath(provider).replace(provider, "{provider}"));
      expect(routes).toContain(oauthCallbackPath(provider).replace(provider, "{provider}"));
    }
  });
});
