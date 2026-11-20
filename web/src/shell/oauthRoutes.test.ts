import { describe, it, expect } from "vitest";
import { oauthBeginPath, oauthCallbackPath } from "./oauthRoutes";

const GO_ROUTES = import.meta.glob("../../../server/internal/api/*.go", {
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
  it("parses both routes out of the internal/api package", () => {
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
